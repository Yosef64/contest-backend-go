"""Turns a Word (.docx) course chapter into a lesson the admin can import.

    python scripts/docx_to_lesson.py "dynamics chapter 4.docx" [-o out.json]
        [--title "Dynamics"] [--course COURSE_ID] [--status draft|published]
        [--split]   (one file per chapter)

Writes the JSON the admin's "Import lessons" panel takes ({"chapters": [...]})
and a preview page (.html) to check the result in a browser. Python 3.9+,
nothing to install.

What it keeps:
- text, bold/italic/underline, superscript/subscript, text colours and
  highlights, English and Amharic;
- chapters: each "CHAPTER ..." line starts a new lesson (its next line is
  the title), so one file can hold several chapters;
- headings, by how they look: bold text larger than the body (20 pt and
  up) becomes a topic (h2, a page in the app); bold 18 pt, or bold and
  underlined, becomes a sub-heading (h3). Word's Heading styles work too;
- numbered and bulleted lists;
- Word equations, as formulas the app draws (LaTeX in data-tex);
- diagrams drawn with Word shapes (boxes, arrows, lines, labels) as SVG.
  Their layout is rebuilt from the shapes' positions, so check them in the
  preview; pictures (photos) are saved next to the JSON to be uploaded.
"""
from __future__ import annotations

import argparse
import html
import json
import math
import re
import sys
import unicodedata
import zipfile
import xml.etree.ElementTree as ET
from pathlib import Path

NS = {
    'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
    'm': 'http://schemas.openxmlformats.org/officeDocument/2006/math',
    'wp': 'http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing',
    'a': 'http://schemas.openxmlformats.org/drawingml/2006/main',
    'wps': 'http://schemas.microsoft.com/office/word/2010/wordprocessingShape',
    'wpg': 'http://schemas.microsoft.com/office/word/2010/wordprocessingGroup',
    'mc': 'http://schemas.openxmlformats.org/markup-compatibility/2006',
    'pic': 'http://schemas.openxmlformats.org/drawingml/2006/picture',
    'r': 'http://schemas.openxmlformats.org/officeDocument/2006/relationships',
}
W, M, WP, A, WPS, MC = (f'{{{NS[k]}}}' for k in ('w', 'm', 'wp', 'a', 'wps', 'mc'))

EMU_PX = 9525           # EMUs per CSS pixel
EMU_PT = 12700          # EMUs per point
LINE_EMU = 260000       # fallback height of one text line (~0.28 in)
CHAR_EMU = 80000        # fallback width of one label character
TAB_EMU = 457200        # default tab stop (0.5 in)


class Layout:
    """Enough of Word's layout to place diagram labels: paragraph heights
    (font size, line spacing, space before/after) and text widths."""

    def __init__(self, z):
        self.after, self.before, self.line, self.size = 0, 0, 240, 22
        self.style_ind = {}
        self.style_ctx = set()
        self.tab = TAB_EMU
        try:
            st = ET.fromstring(z.read('word/styles.xml'))
            for style in st.findall('w:style', NS):
                ppr = style.find('w:pPr', NS)
                sid = style.get(W + 'styleId')
                if ppr is not None and ppr.find('w:ind', NS) is not None:
                    ind = ppr.find('w:ind', NS)
                    self.style_ind[sid] = _int(ind, W + 'left') or _int(ind, W + 'start')
                if ppr is not None and ppr.find('w:contextualSpacing', NS) is not None:
                    self.style_ctx.add(sid)
                if style.get(W + 'default') == '1' and style.get(W + 'type') == 'paragraph':
                    sp = ppr.find('w:spacing', NS) if ppr is not None else None
                    if sp is not None:
                        self.after = _int(sp, W + 'after', self.after)
                        self.before = _int(sp, W + 'before', self.before)
                        self.line = _int(sp, W + 'line', self.line)
                    sz = style.find('w:rPr/w:sz', NS)
                    if sz is not None:
                        self.size = _int(sz, W + 'val', self.size)
        except KeyError:
            pass
        try:
            tab = ET.fromstring(z.read('word/settings.xml')).find('w:defaultTabStop', NS)
            if tab is not None:
                self.tab = _int(tab, W + 'val', 720) * 635
        except KeyError:
            pass

    def run_size(self, r):
        sz = r.find('w:rPr/w:sz', NS)
        return _int(sz, W + 'val', self.size) / 2 if sz is not None else self.size / 2

    def para_size(self, p):
        sizes = [self.run_size(r) for r in p.findall('w:r', NS) if ''.join(t.text or '' for t in r.iter(W + 't'))]
        return max(sizes) if sizes else self.size / 2

    def style(self, p):
        st = p.find('w:pPr/w:pStyle', NS)
        return st.get(W + 'val') if st is not None else ''

    def height(self, p):
        """Height of a one-line paragraph, in EMUs."""
        sp = p.find('w:pPr/w:spacing', NS)
        after = _int(sp, W + 'after', self.after) if sp is not None else self.after
        before = _int(sp, W + 'before', self.before) if sp is not None else self.before
        line = _int(sp, W + 'line', self.line) if sp is not None else self.line
        if self.style(p) in self.style_ctx or p.find('w:pPr/w:contextualSpacing', NS) is not None:
            after = before = 0
        size = self.para_size(p)
        return int((size * 1.22 * line / 240 + (after + before) / 20) * EMU_PT)

    def indent(self, p):
        ind = p.find('w:pPr/w:ind', NS)
        if ind is not None and (ind.get(W + 'left') or ind.get(W + 'start')):
            return (_int(ind, W + 'left') or _int(ind, W + 'start')) * 635
        return self.style_ind.get(self.style(p), 0) * 635


# --- Equations: Office Math (OMML) to LaTeX ------------------------------------

SYMBOLS = {
    'α': r'\alpha', 'β': r'\beta', 'γ': r'\gamma', 'δ': r'\delta', 'ε': r'\varepsilon',
    'ϵ': r'\epsilon', 'ζ': r'\zeta', 'η': r'\eta', 'θ': r'\theta', 'ϑ': r'\vartheta',
    'ι': r'\iota', 'κ': r'\kappa', 'λ': r'\lambda', 'μ': r'\mu', 'ν': r'\nu', 'ξ': r'\xi',
    'π': r'\pi', 'ρ': r'\rho', 'σ': r'\sigma', 'τ': r'\tau', 'υ': r'\upsilon', 'φ': r'\varphi',
    'ϕ': r'\phi', 'χ': r'\chi', 'ψ': r'\psi', 'ω': r'\omega', 'Γ': r'\Gamma', 'Δ': r'\Delta',
    'Θ': r'\Theta', 'Λ': r'\Lambda', 'Ξ': r'\Xi', 'Π': r'\Pi', 'Σ': r'\Sigma', 'Φ': r'\Phi',
    'Ψ': r'\Psi', 'Ω': r'\Omega', '∞': r'\infty', '±': r'\pm', '∓': r'\mp', '×': r'\times',
    '÷': r'\div', '·': r'\cdot', '⋅': r'\cdot', '∙': r'\cdot', '≤': r'\le', '≥': r'\ge',
    '≠': r'\ne', '≈': r'\approx', '≡': r'\equiv', '∝': r'\propto', '→': r'\to',
    '←': r'\leftarrow', '⇒': r'\Rightarrow', '⇔': r'\Leftrightarrow', '↔': r'\leftrightarrow',
    '∑': r'\sum', '∏': r'\prod', '∫': r'\int', '∮': r'\oint', '∂': r'\partial', '∇': r'\nabla',
    '√': r'\surd', '°': r'^{\circ}', '∠': r'\angle', '⊥': r'\perp', '∥': r'\parallel',
    '∈': r'\in', '∉': r'\notin', '⊂': r'\subset', '∪': r'\cup', '∩': r'\cap', '∅': r'\emptyset',
    '…': r'\ldots', '⋯': r'\cdots', '′': "'", '″': "''", '−': '-', '–': '-', '∗': '*',
    'ℏ': r'\hbar', 'ℓ': r'\ell', '∴': r'\therefore', '∵': r'\because', '⟹': r'\Longrightarrow',
}
FUNCS = {'sin', 'cos', 'tan', 'cot', 'sec', 'csc', 'log', 'ln', 'exp', 'lim', 'max', 'min',
         'sinh', 'cosh', 'tanh', 'arcsin', 'arccos', 'arctan', 'det'}
ACCENTS = {'⃗': r'\vec', '̂': r'\hat', '̄': r'\bar', '̅': r'\overline',
           '̇': r'\dot', '̈': r'\ddot', '̃': r'\tilde', '⃖': r'\overleftarrow',
           '→': r'\vec', '^': r'\hat', '~': r'\tilde'}
NARY = {'∑': r'\sum', '∏': r'\prod', '∫': r'\int', '∬': r'\iint', '∭': r'\iiint', '∮': r'\oint',
        '⋃': r'\bigcup', '⋂': r'\bigcap'}
DELIMS = {'(': '(', ')': ')', '[': '[', ']': ']', '{': r'\{', '}': r'\}', '|': '|',
          '‖': r'\|', '⟨': r'\langle', '⟩': r'\rangle', '⌊': r'\lfloor', '⌋': r'\rfloor',
          '⌈': r'\lceil', '⌉': r'\rceil', '': '.'}


def _val(el, path):
    found = el.find(path, NS) if el is not None else None
    return found.get(M + 'val') if found is not None else None


def _tex_text(s: str) -> str:
    out = []
    for ch in s:
        if ch in SYMBOLS:
            out.append(SYMBOLS[ch] + ' ' if SYMBOLS[ch].startswith('\\') and SYMBOLS[ch][1:].isalpha() else SYMBOLS[ch])
        elif ch in '#$%&_{}':
            out.append('\\' + ch)
        elif ch == '\\':
            out.append(r'\backslash ')
        elif ch == '^':
            out.append(r'\hat{}')
        elif ch == '~':
            out.append(r'\sim ')
        elif ch == ' ':
            out.append(r'\ ')
        elif ch == ' ':
            out.append(r'\ ')
        else:
            out.append(ch)
    text = ''.join(out)
    return text


def omml(el) -> str:
    """LaTeX for an OMML element (m:oMath or any part of it)."""
    tag = el.tag.replace(M, 'm:').replace(W, 'w:')
    kids = list(el)

    def sub(name):
        found = el.find('m:' + name, NS)
        return omml(found) if found is not None else ''

    if tag in ('m:oMath', 'm:e', 'm:num', 'm:den', 'm:sub', 'm:sup', 'm:deg', 'm:fName', 'm:lim'):
        return ''.join(omml(k) for k in kids)
    if tag == 'm:r':
        t = ''.join(x.text or '' for x in el.iter(M + 't'))
        if not t:
            t = ''.join(x.text or '' for x in el.iter(W + 't'))
        plain = _val(el, 'm:rPr/m:sty') == 'p' or el.find('m:rPr/m:nor', NS) is not None
        word = t.strip()
        if word in FUNCS:
            return '\\' + word + ' '
        tex = _tex_text(t)
        if plain and re.fullmatch(r'[A-Za-z]{2,}', word):
            return r'\mathrm{' + tex + '}'
        return tex
    if tag == 'm:f':
        num, den = sub('num'), sub('den')
        kind = _val(el, 'm:fPr/m:type')
        if kind == 'lin':
            return f'{num}/{den}'
        if kind == 'noBar':
            return rf'\genfrac{{}}{{}}{{0pt}}{{}}{{{num}}}{{{den}}}'
        return rf'\frac{{{num}}}{{{den}}}'
    if tag == 'm:sSup':
        return f'{{{sub("e")}}}^{{{sub("sup")}}}'
    if tag == 'm:sSub':
        return f'{{{sub("e")}}}_{{{sub("sub")}}}'
    if tag == 'm:sSubSup':
        return f'{{{sub("e")}}}_{{{sub("sub")}}}^{{{sub("sup")}}}'
    if tag == 'm:sPre':
        return f'{{}}_{{{sub("sub")}}}^{{{sub("sup")}}}{{{sub("e")}}}'
    if tag == 'm:rad':
        deg = sub('deg')
        hide = _val(el, 'm:radPr/m:degHide') in ('1', 'on', 'true')
        return rf'\sqrt[{deg}]{{{sub("e")}}}' if deg and not hide else rf'\sqrt{{{sub("e")}}}'
    if tag == 'm:d':
        pr = el.find('m:dPr', NS)
        beg = _val(pr, 'm:begChr')
        end = _val(pr, 'm:endChr')
        sep = _val(pr, 'm:sepChr') or '|'
        beg = '(' if beg is None else beg
        end = ')' if end is None else end
        items = [omml(e) for e in el.findall('m:e', NS)]
        body = f' {DELIMS.get(sep, sep)} '.join(items)
        return rf'\left{DELIMS.get(beg, beg)}{body}\right{DELIMS.get(end, end)}'
    if tag == 'm:nary':
        pr = el.find('m:naryPr', NS)
        ch = _val(pr, 'm:chr') or '∫'
        op = NARY.get(ch, _tex_text(ch))
        lo, hi = sub('sub'), sub('sup')
        return op + (f'_{{{lo}}}' if lo else '') + (f'^{{{hi}}}' if hi else '') + ' ' + sub('e')
    if tag == 'm:acc':
        ch = _val(el, 'm:accPr/m:chr') or '̂'
        return f'{ACCENTS.get(ch, r"\hat")}{{{sub("e")}}}'
    if tag == 'm:bar':
        pos = _val(el, 'm:barPr/m:pos')
        return (r'\underline{' if pos == 'bot' else r'\overline{') + sub('e') + '}'
    if tag == 'm:func':
        name = sub('fName').strip()
        if name.replace(' ', '') in FUNCS:
            name = '\\' + name.replace(' ', '')
        return f'{name} {sub("e")}'
    if tag == 'm:limLow':
        return f'{sub("e")}_{{{sub("lim")}}}'
    if tag == 'm:limUpp':
        return f'{sub("e")}^{{{sub("lim")}}}'
    if tag == 'm:groupChr':
        pos = _val(el, 'm:groupChrPr/m:pos')
        return (r'\overbrace{' if pos == 'top' else r'\underbrace{') + sub('e') + '}'
    if tag == 'm:eqArr':
        rows = [omml(e).replace('&', '') for e in el.findall('m:e', NS)]
        return r'\begin{aligned}' + r' \\ '.join(rows) + r'\end{aligned}'
    if tag == 'm:m':
        rows = [' & '.join(omml(e) for e in mr.findall('m:e', NS)) for mr in el.findall('m:mr', NS)]
        return r'\begin{matrix}' + r' \\ '.join(rows) + r'\end{matrix}'
    if tag in ('m:box', 'm:borderBox', 'm:phant'):
        return sub('e')
    if tag == 'w:r':  # text runs inside equations
        return _tex_text(''.join(x.text or '' for x in el.iter(W + 't')))
    if tag.endswith('Pr') or tag in ('m:ctrlPr', 'w:rPr', 'w:bookmarkStart', 'w:bookmarkEnd', 'w:proofErr'):
        return ''
    return ''.join(omml(k) for k in kids)


def tidy_tex(tex: str) -> str:
    tex = re.sub(r'(\\ ){2,}', r'\\ ', tex)
    # Drop spacing at the ends only (a leading \left must stay).
    return re.sub(r'^(?:\\ |\s)+|(?:\\ |\s)+$', '', tex)


# --- Diagrams: Word shapes to SVG -------------------------------------------------

class Shape:
    def __init__(self, x, y, w, h, geom, line, fill, rot, flip_h, flip_v, head, tail, text):
        self.x, self.y, self.w, self.h = x, y, w, h
        self.geom, self.line, self.fill = geom, line, fill
        self.rot, self.flip_h, self.flip_v = rot, flip_h, flip_v
        self.head, self.tail = head, tail
        self.text, self.size = text if isinstance(text, tuple) else (text, 11)


def _int(el, attr, default=0):
    try:
        return int(el.get(attr))
    except (TypeError, ValueError, AttributeError):
        return default


def _shape_props(sp):
    """Geometry, line and fill of a wps:wsp (or pic) element."""
    xfrm = sp.find('.//a:xfrm', NS)
    rot = _int(xfrm, 'rot') / 60000 if xfrm is not None else 0
    flip_h = xfrm is not None and xfrm.get('flipH') == '1'
    flip_v = xfrm is not None and xfrm.get('flipV') == '1'
    geom_el = sp.find('.//a:prstGeom', NS)
    geom = geom_el.get('prst') if geom_el is not None else ('custom' if sp.find('.//a:custGeom', NS) is not None else 'rect')
    sppr = sp.find('wps:spPr', NS)
    ln = sppr.find('a:ln', NS) if sppr is not None else None
    no_line = ln is not None and ln.find('a:noFill', NS) is not None
    style_line = sp.find('wps:style/a:lnRef', NS)
    has_line = not no_line and (ln is not None or (style_line is not None and _int(style_line, 'idx') > 0))
    width = _int(ln, 'w', 12700) if ln is not None else 12700
    fill_el = sppr.find('a:solidFill', NS) if sppr is not None else None
    no_fill = sppr is not None and sppr.find('a:noFill', NS) is not None
    fill = None
    if fill_el is not None:
        clr = fill_el.find('a:srgbClr', NS)
        fill = '#' + clr.get('val') if clr is not None else '#ffffff'
    elif not no_fill and geom not in ('line', 'straightConnector1') and sp.find('wps:style/a:fillRef', NS) is not None \
            and _int(sp.find('wps:style/a:fillRef', NS), 'idx') > 0:
        fill = '#ffffff'
    head = ln is not None and ln.find('a:headEnd', NS) is not None and ln.find('a:headEnd', NS).get('type', 'none') != 'none'
    tail = ln is not None and ln.find('a:tailEnd', NS) is not None and ln.find('a:tailEnd', NS).get('type', 'none') != 'none'
    text = ' '.join(''.join(t.text or '' for t in p.iter(W + 't')).strip()
                    for p in sp.iter(W + 'p')).strip()
    szs = [_int(x, W + 'val') for x in sp.iter(W + 'sz')]
    size = (max(szs) / 2) if szs else 11
    return geom, (max(width, 9525) if has_line else 0), fill, rot, flip_h, flip_v, head, tail, (text, size)


def anchored_shapes(par, par_y):
    """Shapes anchored to a paragraph, positioned in EMUs within the column."""
    shapes = []
    for anchor in par.iter(WP + 'anchor'):
        ph = anchor.find('wp:positionH', NS)
        pv = anchor.find('wp:positionV', NS)
        ext = anchor.find('wp:extent', NS)
        x = _int(ph.find('wp:posOffset', NS), None) if ph is not None and ph.find('wp:posOffset', NS) is not None else 0
        x = int(ph.find('wp:posOffset', NS).text) if ph is not None and ph.find('wp:posOffset', NS) is not None else 0
        y = int(pv.find('wp:posOffset', NS).text) if pv is not None and pv.find('wp:posOffset', NS) is not None else 0
        if pv is not None and pv.get('relativeFrom') in ('paragraph', 'line'):
            y += par_y
        elif pv is not None:
            y = par_y + y % LINE_EMU  # page/margin anchors: keep near their paragraph
        if ph is not None and ph.get('relativeFrom') in ('page',):
            x -= 914400  # page edge to the text column (about 1 in margin)
        cx, cy = _int(ext, 'cx'), _int(ext, 'cy')
        sp = anchor.find('.//wps:wsp', NS)
        if sp is not None:
            shapes.append(Shape(x, y, cx, cy, *_shape_props(sp)))
        elif anchor.find('.//pic:pic', NS) is not None:
            shapes.append(Shape(x, y, cx, cy, 'picture', 9525, '#eef2f6', 0, False, False, False, False, 'picture'))
    return shapes


def _rotate(px, py, cx, cy, deg):
    if not deg:
        return px, py
    r = math.radians(deg)
    dx, dy = px - cx, py - cy
    return cx + dx * math.cos(r) - dy * math.sin(r), cy + dx * math.sin(r) + dy * math.cos(r)


def _arrowhead(x1, y1, x2, y2, size):
    """Triangle at (x2, y2) pointing away from (x1, y1)."""
    ang = math.atan2(y2 - y1, x2 - x1)
    left = (x2 - size * math.cos(ang - 0.45), y2 - size * math.sin(ang - 0.45))
    right = (x2 - size * math.cos(ang + 0.45), y2 - size * math.sin(ang + 0.45))
    return f'<polygon points="{x2:.1f},{y2:.1f} {left[0]:.1f},{left[1]:.1f} {right[0]:.1f},{right[1]:.1f}" fill="#1f2933"/>'


def svg_for(shapes, labels):
    """An SVG drawing: shapes, plus loose text labels [(x, y, text)] in EMUs."""
    items = []
    xs, ys = [], []
    for s in shapes:
        xs += [s.x, s.x + s.w]
        ys += [s.y, s.y + s.h]
    for x, y, t, size in labels:
        xs += [x, x + len(t) * size * 0.55 * EMU_PT]
        ys += [y - size * EMU_PT, y + size * 0.3 * EMU_PT]
    if not xs:
        return ''
    pad = 120000
    x0, y0 = min(xs) - pad, min(ys) - pad
    width, height = max(xs) - x0 + pad, max(ys) - y0 + pad

    def px(v):
        return v / EMU_PX

    for s in shapes:
        x, y, w, h = px(s.x - x0), px(s.y - y0), px(s.w), px(s.h)
        cx, cy = x + w / 2, y + h / 2
        stroke = px(s.line) if s.line else 0
        attrs = f'stroke="#1f2933" stroke-width="{max(stroke, 1.2):.1f}"' if s.line else 'stroke="none"'
        fill = s.fill or 'none'
        if s.geom in ('line', 'straightConnector1', 'curvedConnector3', 'bentConnector3', 'rightArrow',
                      'leftArrow', 'upArrow', 'downArrow') and s.geom.endswith('Arrow') is False or s.geom in ('line', 'straightConnector1'):
            x1, y1, x2, y2 = x, y, x + w, y + h
            if s.flip_h:
                x1, x2 = x2, x1
            if s.flip_v:
                y1, y2 = y2, y1
            x1, y1 = _rotate(x1, y1, cx, cy, s.rot)
            x2, y2 = _rotate(x2, y2, cx, cy, s.rot)
            sw = max(stroke, 1.5)
            items.append(f'<line x1="{x1:.1f}" y1="{y1:.1f}" x2="{x2:.1f}" y2="{y2:.1f}" stroke="#1f2933" stroke-width="{sw:.1f}"/>')
            size = 6 + sw * 2
            if s.tail:
                items.append(_arrowhead(x1, y1, x2, y2, size))
            if s.head:
                items.append(_arrowhead(x2, y2, x1, y1, size))
            continue
        if s.geom in ('rightArrow', 'leftArrow', 'upArrow', 'downArrow'):
            # Block arrows: a thick line with a head, in their direction.
            ends = {'rightArrow': (x, cy, x + w, cy), 'leftArrow': (x + w, cy, x, cy),
                    'upArrow': (cx, y + h, cx, y), 'downArrow': (cx, y, cx, y + h)}[s.geom]
            x1, y1 = _rotate(ends[0], ends[1], cx, cy, s.rot)
            x2, y2 = _rotate(ends[2], ends[3], cx, cy, s.rot)
            items.append(f'<line x1="{x1:.1f}" y1="{y1:.1f}" x2="{x2:.1f}" y2="{y2:.1f}" stroke="#1f2933" stroke-width="2"/>')
            items.append(_arrowhead(x1, y1, x2, y2, 10))
            continue
        rot = f' transform="rotate({s.rot:.1f} {cx:.1f} {cy:.1f})"' if s.rot else ''
        if s.geom in ('ellipse', 'flowChartConnector'):
            items.append(f'<ellipse cx="{cx:.1f}" cy="{cy:.1f}" rx="{w / 2:.1f}" ry="{h / 2:.1f}" fill="{fill}" {attrs}{rot}/>')
        elif s.geom in ('triangle', 'rtTriangle'):
            pts = f'{cx:.1f},{y:.1f} {x + w:.1f},{y + h:.1f} {x:.1f},{y + h:.1f}' if s.geom == 'triangle' else \
                f'{x:.1f},{y:.1f} {x + w:.1f},{y + h:.1f} {x:.1f},{y + h:.1f}'
            if s.flip_h and s.geom == 'rtTriangle':
                pts = f'{x + w:.1f},{y:.1f} {x:.1f},{y + h:.1f} {x + w:.1f},{y + h:.1f}'
            items.append(f'<polygon points="{pts}" fill="{fill}" {attrs}{rot}/>')
        elif s.geom == 'picture':
            items.append(f'<rect x="{x:.1f}" y="{y:.1f}" width="{w:.1f}" height="{h:.1f}" fill="#eef2f6" stroke="#9aa0a6" stroke-dasharray="4 3"/>')
        elif s.line or s.fill:
            r = min(w, h) * 0.15 if s.geom in ('roundRect', 'flowChartAlternateProcess') else 0
            items.append(f'<rect x="{x:.1f}" y="{y:.1f}" width="{w:.1f}" height="{h:.1f}" rx="{r:.1f}" fill="{fill}" {attrs}{rot}/>')
        if s.text and s.geom != 'picture':
            size = s.size * EMU_PT / EMU_PX
            items.append(f'<text x="{cx:.1f}" y="{cy + size * 0.35:.1f}" font-size="{size:.0f}" text-anchor="middle" '
                         f'font-family="sans-serif" fill="#1f2933">{html.escape(s.text)}</text>')
    for x, y, t, size in labels:
        items.append(f'<text x="{px(x - x0):.1f}" y="{px(y - y0):.1f}" font-size="{size * EMU_PT / EMU_PX:.0f}" font-family="sans-serif" '
                     f'fill="#1f2933">{html.escape(t)}</text>')
    vw, vh = px(width), px(height)
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {vw:.0f} {vh:.0f}" width="{min(vw, 560):.0f}" '
            f'height="{vh * min(vw, 560) / vw:.0f}">' + ''.join(items) + '</svg>')


# --- Paragraphs ------------------------------------------------------------------

ETHIOPIC = re.compile(r'[ሀ-፿]')


def run_html(r) -> str:
    rpr = r.find('w:rPr', NS)
    parts = []
    for node in r:
        if node.tag == W + 't':
            parts.append(html.escape(node.text or ''))
        elif node.tag == W + 'tab':
            parts.append(' ')
        elif node.tag in (W + 'br', W + 'cr'):
            parts.append('<br>')
        elif node.tag == W + 'sym':
            code = node.get(W + 'char', '')
            try:
                parts.append(html.escape(chr(int(code, 16) % 0xF000 if int(code, 16) >= 0xF000 else int(code, 16))))
            except ValueError:
                pass
    text = ''.join(parts)
    if not text.strip():
        return text
    if rpr is not None:
        def on(tag):
            el = rpr.find('w:' + tag, NS)
            return el is not None and el.get(W + 'val') not in ('0', 'false', 'none')
        va = rpr.find('w:vertAlign', NS)
        if va is not None and va.get(W + 'val') == 'superscript':
            text = f'<sup>{text}</sup>'
        elif va is not None and va.get(W + 'val') == 'subscript':
            text = f'<sub>{text}</sub>'
        if on('b'):
            text = f'<strong>{text}</strong>'
        if on('i'):
            text = f'<em>{text}</em>'
        if on('u'):
            text = f'<u>{text}</u>'
        style = run_colours(rpr)
        if style:
            text = f'<span style="{style}">{text}</span>'
    return text


# Word's highlight names, as hex (what the lesson styles accept).
HIGHLIGHTS = {
    'yellow': '#FFFF00', 'green': '#00FF00', 'cyan': '#00FFFF', 'magenta': '#FF00FF',
    'blue': '#0000FF', 'red': '#FF0000', 'darkBlue': '#000080', 'darkCyan': '#008080',
    'darkGreen': '#008000', 'darkMagenta': '#800080', 'darkRed': '#800000',
    'darkYellow': '#808000', 'darkGray': '#808080', 'lightGray': '#C0C0C0', 'black': '#000000',
}


def run_colours(rpr) -> str:
    """Inline style for a run's text colour and highlight/shading, if any.
    Black and "automatic" text are left alone so the app's own colours
    (including dark mode) apply."""
    parts = []
    color = rpr.find('w:color', NS)
    val = (color.get(W + 'val') or '') if color is not None else ''
    if re.fullmatch(r'[0-9A-Fa-f]{6}', val) and val.upper() not in ('000000', 'FFFFFF'):
        parts.append(f'color: #{val.upper()}')
    mark = rpr.find('w:highlight', NS)
    shade = rpr.find('w:shd', NS)
    back = ''
    if mark is not None and mark.get(W + 'val') in HIGHLIGHTS:
        back = HIGHLIGHTS[mark.get(W + 'val')]
    elif shade is not None and re.fullmatch(r'[0-9A-Fa-f]{6}', shade.get(W + 'fill') or '') \
            and shade.get(W + 'fill').upper() not in ('FFFFFF', 'AUTO'):
        back = '#' + shade.get(W + 'fill').upper()
    if back:
        parts.append(f'background-color: {back}')
    return '; '.join(parts)


def para_content(p):
    """(html, plain text, equations) of a paragraph, skipping shape text."""
    out, plain, maths = [], [], []

    def walk(el):
        for node in el:
            t = node.tag
            if t == W + 'r':
                if node.find('.//w:drawing', NS) is not None or node.find('.//mc:AlternateContent', NS) is not None \
                        or node.find('w:pict', NS) is not None:
                    continue
                h = run_html(node)
                out.append(h)
                plain.append(''.join(x.text or '' for x in node.iter(W + 't')))
            elif t == M + 'oMathPara':
                for om in node.iter(M + 'oMath'):
                    tex = tidy_tex(omml(om))
                    if not tex:
                        continue  # an empty equation box
                    maths.append(tex)
                    out.append(f'\x00DISPLAY{len(maths) - 1}\x00')
            elif t == M + 'oMath':
                tex = tidy_tex(omml(node))
                if not tex:
                    continue
                maths.append(tex)
                out.append(f'\x00INLINE{len(maths) - 1}\x00')
                plain.append(' ⟦m⟧ ')
            elif t in (W + 'hyperlink', W + 'smartTag', W + 'ins', W + 'sdt', W + 'sdtContent', W + 'fldSimple'):
                walk(node)
            elif t == MC + 'AlternateContent':
                continue
    walk(p)
    return ''.join(out), ''.join(plain), maths


def render_inline(h, maths):
    def rep(m):
        kind, i = m.group(1), int(m.group(2))
        tex = html.escape(maths[i], quote=True)
        if kind == 'DISPLAY':
            return f'</p><div data-tex="{tex}"></div><p>'
        return f'<span data-tex="{tex}"></span>'
    return re.sub(r'\x00(INLINE|DISPLAY)(\d+)\x00', rep, h)


def list_kinds(z):
    """numId -> 'ol' or 'ul' (from numbering.xml, first level)."""
    try:
        root = ET.fromstring(z.read('word/numbering.xml'))
    except KeyError:
        return {}
    abstract = {}
    for an in root.findall('w:abstractNum', NS):
        lvl = an.find('w:lvl', NS)
        fmt = lvl.find('w:numFmt', NS).get(W + 'val') if lvl is not None and lvl.find('w:numFmt', NS) is not None else 'bullet'
        abstract[an.get(W + 'abstractNumId')] = 'ul' if fmt in ('bullet', 'none') else 'ol'
    kinds = {}
    for num in root.findall('w:num', NS):
        aid = num.find('w:abstractNumId', NS)
        kinds[num.get(W + 'numId')] = abstract.get(aid.get(W + 'val') if aid is not None else '', 'ul')
    return kinds


LABELS = re.compile(r'^(solution|given|required|answer|example|examples|note|therefore|so)\b', re.I)


def para_format(p):
    """(all text bold, largest size in half-points, all underlined)."""
    bold = underline = True
    size = 0
    seen = False
    for r in p.findall('w:r', NS):
        if r.find('.//w:drawing', NS) is not None or r.find('.//mc:AlternateContent', NS) is not None:
            continue
        if not ''.join(t.text or '' for t in r.iter(W + 't')).strip():
            continue
        seen = True
        rpr = r.find('w:rPr', NS)

        def on(tag):
            el = rpr.find('w:' + tag, NS) if rpr is not None else None
            return el is not None and el.get(W + 'val') not in ('0', 'false', 'none')
        bold = bold and on('b')
        underline = underline and on('u')
        sz = rpr.find('w:sz', NS) if rpr is not None else None
        if sz is not None:
            size = max(size, _int(sz, W + 'val'))
    return (bold and seen), size, (underline and seen)


def heading_level(p, text, has_math, body_size):
    t = text.strip()
    # Not headings: labels, sentences, exclamations (mottos), formulas.
    if not t or has_math or len(t) > 100 or LABELS.match(t) or t.endswith(('.', '!', ':')) \
            or '=' in t or t.startswith(('=>', '-', '*')):
        return 0
    bold, size, underline = para_format(p)
    if not bold:
        return 0
    numbered = re.match(r'^\d+\s*[.)]\s*\S', t) is not None
    if size >= body_size + 8 or (numbered and size >= body_size + 4):
        return 2                   # 20 pt and up, or "1. Work Done" at 18 pt
    if size >= body_size + 4 or underline or re.match(r'^\d+\s*[,.)]\s*[A-Z]', t):
        return 3                   # also "1, FORCE OF GRAVITY" in bold
    return 0


def body_size_of(body):
    """The most common text size (half-points): the body text."""
    counts = {}
    for r in body.iter(W + 'r'):
        sz = r.find('w:rPr/w:sz', NS)
        n = len(''.join(t.text or '' for t in r.iter(W + 't')))
        if sz is not None and n:
            counts[_int(sz, W + 'val')] = counts.get(_int(sz, W + 'val'), 0) + n
    return max(counts, key=counts.get) if counts else 22


def convert(path: Path, title: str | None, pictures_dir: Path | None = None):
    z = zipfile.ZipFile(path)
    body = ET.fromstring(z.read('word/document.xml')).find(W + 'body')
    try:
        rels = {r.get('Id'): r.get('Target') for r in
                ET.fromstring(z.read('word/_rels/document.xml.rels'))}
    except KeyError:
        rels = {}
    kinds = list_kinds(z)
    body_size = body_size_of(body)
    layout = Layout(z)
    chapters = []    # [title, html pieces]
    blocks = []      # html pieces of the chapter being read
    report = {'equations': 0, 'diagrams': 0, 'pictures': 0, 'headings': [], 'unclear_equations': []}
    chapter_title = title
    want_title = False

    # Diagram state: shapes and short label lines collected over consecutive paragraphs.
    diagram, labels, diag_y, pending_gap = [], [], 0, 0
    list_open = None

    def flush_diagram():
        nonlocal diagram, labels, diag_y, pending_gap
        if diagram:
            svg = svg_for(diagram, labels)
            if svg:
                report['diagrams'] += 1
                blocks.append(f'<figure>{svg}<figcaption>Diagram {report["diagrams"]}</figcaption></figure>')
        diagram, labels, diag_y, pending_gap = [], [], 0, 0

    def close_list():
        nonlocal list_open
        if list_open:
            blocks.append(f'</{list_open}>')
            list_open = None

    paragraphs = [el for el in body if el.tag in (W + 'p', W + 'tbl')]
    for idx, el in enumerate(paragraphs):
        if el.tag == W + 'tbl':
            flush_diagram()
            close_list()
            rows = []
            for tr in el.findall('w:tr', NS):
                cells = []
                for tc in tr.findall('w:tc', NS):
                    inner = []
                    for p in tc.findall('w:p', NS):
                        h, _, maths = para_content(p)
                        report['equations'] += len(maths)
                        inner.append(render_inline(h, maths))
                    cells.append('<td>' + '<br>'.join(x for x in inner if x.strip()) + '</td>')
                rows.append('<tr>' + ''.join(cells) + '</tr>')
            blocks.append('<table>' + ''.join(rows) + '</table>')
            continue

        p = el
        h, plain, maths = para_content(p)
        has_shapes = p.find('.//wp:anchor', NS) is not None
        inline_pics = p.findall('.//wp:inline', NS)
        text = re.sub(r'\s+', ' ', plain).strip()

        # Diagram paragraphs: shapes, and the short labels typed between them.
        if has_shapes:
            if not diagram:
                diag_y = 0
            diagram += anchored_shapes(p, diag_y)
            raw = ''.join(x.text or '' for r in p.findall('w:r', NS) for x in r.iter(W + 't')
                          if r.find('.//w:drawing', NS) is None and r.find('.//mc:AlternateContent', NS) is None)
            if _is_label(raw) and not maths:
                labels += _label_tokens(p, diag_y, layout)
                diag_y += layout.height(p)
                continue
            diag_y += layout.height(p)
            if not text and not maths:
                continue
        elif diagram:
            short = _is_label(text) and not maths
            nxt = paragraphs[idx + 1] if idx + 1 < len(paragraphs) else None
            nxt2 = paragraphs[idx + 2] if idx + 2 < len(paragraphs) else None
            more = any(n is not None and n.tag == W + 'p' and n.find('.//wp:anchor', NS) is not None for n in (nxt, nxt2))
            if short or (not text and not maths and more):
                if short:
                    labels += _label_tokens(p, diag_y, layout)
                diag_y += layout.height(p)
                continue
            flush_diagram()

        # Real pictures (photos, scans): saved as files to upload in the editor.
        for inl in inline_pics:
            blip = inl.find('.//a:blip', NS)
            if blip is not None:
                report['pictures'] += 1
                name = f'picture-{report["pictures"]}'
                target = rels.get(blip.get(f'{{{NS["r"]}}}embed', ''), '')
                if pictures_dir and target:
                    pictures_dir.mkdir(parents=True, exist_ok=True)
                    name += Path(target).suffix
                    (pictures_dir / name).write_bytes(z.read('word/' + target.lstrip('/').removeprefix('word/')))
                blocks.append(f'<figure><p class="caption">Picture {report["pictures"]} ({html.escape(name)}): '
                              f'upload it in the editor and put it here.</p></figure>')

        if not text and not maths:
            continue
        report['equations'] += len(maths)
        for tex in maths:
            bare = re.sub(r'\\[{}]', '', tex)  # \{ and \} are shown braces
            if not tex or bare.count('{') != bare.count('}'):
                report['unclear_equations'].append(tex)

        # "CHAPTER ..." starts a lesson; the next line is its title.
        if re.match(r'^chapter\b', text, re.I) and len(text) < 40:
            flush_diagram()
            close_list()
            if blocks:
                chapters.append([chapter_title, blocks])
            blocks = []
            chapter_title = None
            want_title = True
            report['headings'].append(text)
            continue
        if want_title and len(text) < 80 and not maths:
            chapter_title = title if title and not chapters else text
            want_title = False
            report['headings'].append('  = ' + text)
            continue

        ppr = p.find('w:pPr', NS)
        num = ppr.find('w:numPr', NS) if ppr is not None else None
        style = ppr.find('w:pStyle', NS).get(W + 'val') if ppr is not None and ppr.find('w:pStyle', NS) is not None else ''
        level = heading_level(p, text, bool(maths), body_size)
        if style.lower().startswith('heading'):
            level = 2 if style[-1:] in ('1', '2') else 3
        body_html = render_inline(h, maths).strip()
        if level:
            close_list()
            report['headings'].append(('      ' if level == 3 else '    ') + text)
            blocks.append(f'<h{level}>{html.escape(text)}</h{level}>')
            continue
        if re.fullmatch(r'[A-Z][A-Z \-]{6,}!', text):  # mottos
            close_list()
            blocks.append(f'<div class="callout key"><p><strong>{html.escape(text)}</strong></p></div>')
            continue
        only_math = maths and not re.sub(r'\x00(INLINE|DISPLAY)\d+\x00', '', h).strip()
        if num is not None and not only_math:
            kind = kinds.get(num.find('w:numId', NS).get(W + 'val') if num.find('w:numId', NS) is not None else '', 'ul')
            if list_open != kind:
                close_list()
                blocks.append(f'<{kind}>')
                list_open = kind
            blocks.append(f'<li>{body_html}</li>')
            continue
        close_list()
        if only_math and len(maths) == 1:
            blocks.append(f'<div data-tex="{html.escape(maths[0], quote=True)}"></div>')
        elif only_math:
            # Several equations side by side on one line (worked steps).
            blocks.append('<p>' + ' &nbsp;&nbsp; '.join(f'<span data-tex="{html.escape(t, quote=True)}"></span>'
                                                         for t in maths) + '</p>')
        else:
            blocks.append(f'<p>{body_html}</p>'.replace('<p></p>', ''))
    flush_diagram()
    close_list()
    if blocks:
        chapters.append([chapter_title, blocks])
    out = []
    for i, (name, parts) in enumerate(chapters):
        page = '\n'.join(b for b in parts if b and b != '<p></p>')
        page = re.sub(r'<p>\s*</p>', '', page)
        out.append((name or f'{path.stem} {i + 1}', page))
    return out, report


def _label_tokens(p, y, layout):
    """Loose label text of a diagram line, placed as Word lays it out."""
    x = layout.indent(p)
    tokens, cur, cur_x, cur_size = [], '', x, layout.para_size(p)
    for r in p.findall('w:r', NS):
        if r.find('.//w:drawing', NS) is not None or r.find('.//mc:AlternateContent', NS) is not None:
            continue
        size = layout.run_size(r)
        for node in r:
            if node.tag == W + 'tab':
                if cur.strip():
                    tokens.append((cur_x, cur.strip(), cur_size))
                x = (x // layout.tab + 1) * layout.tab
                cur, cur_x = '', x
            elif node.tag == W + 't':
                for ch in node.text or '':
                    if ch in ' \u00a0':
                        if cur.strip():
                            tokens.append((cur_x, cur.strip(), cur_size))
                            cur = ''
                        x += size * 0.226 * EMU_PT
                        cur_x = x
                    else:
                        if not cur:
                            cur_x, cur_size = x, size
                        cur += ch
                        x += size * 0.5 * EMU_PT
    if cur.strip():
        tokens.append((cur_x, cur.strip(), cur_size))
    base = y + layout.para_size(p) * 1.0 * EMU_PT
    return [(tx, base, t, sz) for tx, t, sz in tokens]


def _is_label(text):
    """Short, label-like text (force names, masses), whatever its spacing."""
    compact = re.sub(r'\s+', ' ', text).strip()
    return bool(compact) and len(compact.replace(' ', '')) <= 30 and len(compact.split()) <= 8 \
        and not ETHIOPIC.search(compact) and not LABELS.match(compact) and not compact.endswith(':')


PREVIEW = """<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>{title}</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/katex@0.16.11/dist/katex.min.css">
<script src="https://cdn.jsdelivr.net/npm/katex@0.16.11/dist/katex.min.js"></script>
<style>body{{font-family:system-ui,'Noto Sans Ethiopic',sans-serif;max-width:720px;margin:24px auto;padding:0 16px;
line-height:1.6;color:#1f2933;background:#fff}}h2{{margin-top:2em;border-top:1px solid #e5e7eb;padding-top:1em}}
figure{{margin:16px 0;padding:12px;border:1px solid #e5e7eb;border-radius:12px;background:#fafafa}}
figcaption,.caption{{color:#71777f;font-size:13px}}div[data-tex]{{margin:8px 0;overflow-x:auto}}
.callout{{background:#eef7f1;border-radius:12px;padding:4px 16px}}.bad{{color:#b91c1c}}</style></head>
<body>{body}
<script>for(const el of document.querySelectorAll('[data-tex]')){{try{{katex.render(el.dataset.tex,el,
{{displayMode:el.tagName==='DIV',throwOnError:true}})}}catch(e){{el.textContent=el.dataset.tex;el.className='bad';
el.title=e.message}}}}</script></body></html>"""


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('docx', type=Path)
    ap.add_argument('-o', '--out', type=Path)
    ap.add_argument('--title')
    ap.add_argument('--course', help='course id (or choose the course in the admin)')
    ap.add_argument('--status', default='draft', choices=['draft', 'published'])
    ap.add_argument('--split', action='store_true', help='one file per chapter')
    args = ap.parse_args()
    sys.stdout.reconfigure(encoding='utf-8')

    out = args.out or args.docx.with_suffix('.lesson.json')
    pictures = out.with_name(out.stem.removesuffix('.lesson') + ' pictures')
    lessons, report = convert(args.docx, args.title, pictures)
    chapters = []
    for title, page in lessons:
        words = len(re.sub(r'<[^>]+>', ' ', page).split())
        first = re.sub(r'<[^>]+>', ' ', page.split('</p>')[0]).strip()
        chapter = {
            'title': title,
            'summary': unicodedata.normalize('NFC', re.sub(r'\s+', ' ', first))[:200],
            'minutes': max(5, min(240, round(words / 100))),
            'status': args.status,
            'html': page,
        }
        if args.course:
            chapter['course_id'] = args.course
        chapters.append(chapter)
    # One file per chapter (--split), or all chapters in one file.
    groups = [[c] for c in chapters] if args.split and len(chapters) > 1 else [chapters]
    written = []
    for group in groups:
        target = out
        if len(groups) > 1:
            safe = re.sub(r'[\\/:*?"<>|]+', ' ', group[0]['title']).strip()
            target = out.with_name(f'{out.stem.removesuffix(".lesson")} - {safe}.lesson.json')
        target.write_text(json.dumps({'chapters': group}, ensure_ascii=False, indent=1), encoding='utf-8')
        preview = target.with_suffix('.preview.html')
        body = ''.join(f'<h1>{html.escape(c["title"])}</h1>{c["html"]}' for c in group)
        preview.write_text(PREVIEW.format(title=html.escape(group[0]['title']), body=body), encoding='utf-8')
        written.append((target, preview))

    for c in chapters:
        topics = c['html'].count('<h2>')
        print(f'Lesson: {c["title"]}  ({c["minutes"]} min read, {topics} topics, {len(c["html"]) // 1024} KB)')
    print(f'Equations: {report["equations"]}, diagrams: {report["diagrams"]}, pictures to upload: {report["pictures"]}'
          + (f' (saved in "{pictures}")' if report['pictures'] else ''))
    print('Outline:')
    for h in report['headings']:
        print('  ' + h)
    if report['unclear_equations']:
        print(f'{len(report["unclear_equations"])} equations may need a look (marked red in the preview).')
    for target, preview in written:
        print(f'Wrote {target}\n  and {preview} (open it in a browser to check)')


if __name__ == '__main__':
    main()
