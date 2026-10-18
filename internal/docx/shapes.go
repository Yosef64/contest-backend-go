package docx

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Diagrams drawn with Word shapes (boxes, arrows, lines, text boxes) and the
// short labels typed between them, rebuilt as SVG from their positions.

const (
	emuPX   = 9525.0   // EMUs per CSS pixel
	emuPT   = 12700.0  // EMUs per point
	tabEMU  = 457200.0 // default tab stop (0.5 in)
	padEMU  = 120000.0
	maxSvgW = 560.0
)

func atoi(s string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return v
	}
	return def
}

// layout knows enough of Word's layout to place diagram labels: paragraph
// heights (font size, line spacing, space before and after) and text widths.
type layout struct {
	after, before, line, size int // twentieths of a point; half-points
	styleInd                  map[string]int
	styleCtx                  map[string]bool
	tab                       float64
}

func newLayout(styles, settings *node) *layout {
	l := &layout{line: 240, size: 22, styleInd: map[string]int{}, styleCtx: map[string]bool{}, tab: tabEMU}
	if styles != nil {
		for _, st := range styles.all("w:style") {
			ppr := st.child("w:pPr")
			id := st.attr("w:styleId")
			if ind := ppr.child("w:ind"); ind != nil {
				l.styleInd[id] = atoi(firstOf(ind.attr("w:left"), ind.attr("w:start")), 0)
			}
			if ppr.child("w:contextualSpacing") != nil {
				l.styleCtx[id] = true
			}
			if st.attr("w:default") == "1" && st.attr("w:type") == "paragraph" {
				if sp := ppr.child("w:spacing"); sp != nil {
					l.after = atoi(sp.attr("w:after"), l.after)
					l.before = atoi(sp.attr("w:before"), l.before)
					l.line = atoi(sp.attr("w:line"), l.line)
				}
				if sz := st.child("w:rPr/w:sz"); sz != nil {
					l.size = atoi(sz.attr("w:val"), l.size)
				}
			}
		}
	}
	if settings != nil {
		if tab := settings.child("w:defaultTabStop"); tab != nil {
			l.tab = float64(atoi(tab.attr("w:val"), 720)) * 635
		}
	}
	return l
}

func firstOf(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func (l *layout) runSize(r *node) float64 {
	if sz := r.child("w:rPr/w:sz"); sz != nil {
		return float64(atoi(sz.attr("w:val"), l.size)) / 2
	}
	return float64(l.size) / 2
}

func (l *layout) paraSize(p *node) float64 {
	best := 0.0
	for _, r := range p.children("w:r") {
		if r.texts("w:t") != "" {
			best = math.Max(best, l.runSize(r))
		}
	}
	if best == 0 {
		return float64(l.size) / 2
	}
	return best
}

func styleOf(p *node) string { return p.val("w:pPr/w:pStyle") }

// height of a one-line paragraph, in EMUs.
func (l *layout) height(p *node) float64 {
	after, before, line := l.after, l.before, l.line
	if sp := p.child("w:pPr/w:spacing"); sp != nil {
		after = atoi(sp.attr("w:after"), after)
		before = atoi(sp.attr("w:before"), before)
		line = atoi(sp.attr("w:line"), line)
	}
	if l.styleCtx[styleOf(p)] || p.child("w:pPr/w:contextualSpacing") != nil {
		after, before = 0, 0
	}
	size := l.paraSize(p)
	return (size*1.22*float64(line)/240 + float64(after+before)/20) * emuPT
}

func (l *layout) indent(p *node) float64 {
	if ind := p.child("w:pPr/w:ind"); ind != nil {
		if v := firstOf(ind.attr("w:left"), ind.attr("w:start")); v != "" {
			return float64(atoi(v, 0)) * 635
		}
	}
	return float64(l.styleInd[styleOf(p)]) * 635
}

type shape struct {
	x, y, w, h   float64
	geom         string
	line         float64
	fill         string
	rot          float64
	flipH, flipV bool
	head, tail   bool
	text         string
	size         float64
	picture      bool
}

type label struct {
	x, y float64
	text string
	size float64
}

// skipRun: drawings and their fallbacks are not text of the paragraph.
func skipRun(r *node) bool {
	return r.find("w:drawing") != nil || r.find("mc:AlternateContent") != nil || r.child("w:pict") != nil
}

func shapeProps(sp *node, s *shape) {
	if x := sp.find("a:xfrm"); x != nil {
		s.rot = float64(atoi(x.attr("rot"), 0)) / 60000
		s.flipH, s.flipV = x.attr("flipH") == "1", x.attr("flipV") == "1"
	}
	s.geom = "rect"
	if g := sp.find("a:prstGeom"); g != nil {
		s.geom = g.attr("prst")
	} else if sp.find("a:custGeom") != nil {
		s.geom = "custom"
	}
	sppr := sp.child("wps:spPr")
	ln := sppr.child("a:ln")
	noLine := ln != nil && ln.child("a:noFill") != nil
	lnRef := sp.child("wps:style/a:lnRef")
	hasLine := !noLine && (ln != nil || (lnRef != nil && atoi(lnRef.attr("idx"), 0) > 0))
	width := 12700.0
	if ln != nil {
		width = float64(atoi(ln.attr("w"), 12700))
	}
	if hasLine {
		s.line = math.Max(width, 9525)
	}
	if fill := sppr.child("a:solidFill"); fill != nil {
		s.fill = "#ffffff"
		if c := fill.child("a:srgbClr"); c != nil {
			s.fill = "#" + c.attr("val")
		}
	} else if sppr.child("a:noFill") == nil && s.geom != "line" && s.geom != "straightConnector1" {
		if fr := sp.child("wps:style/a:fillRef"); fr != nil && atoi(fr.attr("idx"), 0) > 0 {
			s.fill = "#ffffff"
		}
	}
	if ln != nil {
		if h := ln.child("a:headEnd"); h != nil && firstOf(h.attr("type"), "none") != "none" {
			s.head = true
		}
		if t := ln.child("a:tailEnd"); t != nil && firstOf(t.attr("type"), "none") != "none" {
			s.tail = true
		}
	}
	var parts []string
	for _, p := range sp.all("w:p") {
		if t := strings.TrimSpace(p.texts("w:t")); t != "" {
			parts = append(parts, t)
		}
	}
	s.text = strings.Join(parts, " ")
	// Text size of the box's text (largest), 11 pt when unset.
	s.size = 0
	for _, sz := range sp.all("w:sz") {
		s.size = math.Max(s.size, float64(atoi(sz.attr("w:val"), 22))/2)
	}
	if s.size == 0 {
		s.size = 11
	}
}

// anchoredShapes lists the shapes anchored to a paragraph, positioned in EMUs
// within the text column; parY is the paragraph's top in the diagram.
func anchoredShapes(p *node, parY float64) []shape {
	var out []shape
	for _, anchor := range p.all("wp:anchor") {
		ph, pv, ext := anchor.child("wp:positionH"), anchor.child("wp:positionV"), anchor.child("wp:extent")
		x, y := 0.0, 0.0
		if off := ph.child("wp:posOffset"); off != nil {
			x = float64(atoi(off.text, 0))
		}
		if off := pv.child("wp:posOffset"); off != nil {
			y = float64(atoi(off.text, 0))
		}
		switch rel := pv.attr("relativeFrom"); {
		case pv == nil:
		case rel == "paragraph" || rel == "line":
			y += parY
		default:
			y = parY + math.Mod(y, 260000) // page anchors: keep near their paragraph
		}
		if ph != nil && ph.attr("relativeFrom") == "page" {
			x -= 914400 // page edge to the text column (about 1 in margin)
		}
		s := shape{x: x, y: y, w: float64(atoi(ext.attr("cx"), 0)), h: float64(atoi(ext.attr("cy"), 0))}
		if sp := anchor.find("wps:wsp"); sp != nil {
			shapeProps(sp, &s)
			out = append(out, s)
		} else if anchor.find("pic:pic") != nil {
			s.geom, s.picture, s.line = "picture", true, 9525
			out = append(out, s)
		}
	}
	return out
}

// labelTokens places the loose label text of a diagram line as Word lays
// it out: indentation, tab stops, and text and space widths.
func labelTokens(p *node, y float64, l *layout) []label {
	x := l.indent(p)
	var out []label
	cur, curX, curSize := "", x, l.paraSize(p)
	flush := func() {
		if t := strings.TrimSpace(cur); t != "" {
			out = append(out, label{x: curX, text: t, size: curSize})
		}
		cur = ""
	}
	for _, r := range p.children("w:r") {
		if skipRun(r) {
			continue
		}
		size := l.runSize(r)
		for _, k := range r.kids {
			switch {
			case k.is("w:tab"):
				flush()
				x = (math.Floor(x/l.tab) + 1) * l.tab
				curX = x
			case k.is("w:t"):
				for _, ch := range k.text {
					if ch == ' ' || ch == ' ' {
						flush()
						x += size * 0.226 * emuPT
						curX = x
						continue
					}
					if cur == "" {
						curX, curSize = x, size
					}
					cur += string(ch)
					x += size * 0.5 * emuPT
				}
			}
		}
	}
	flush()
	base := y + l.paraSize(p)*emuPT
	for i := range out {
		out[i].y = base
	}
	return out
}

var spaces = regexp.MustCompile(`\s+`)

// isLabel: short, label-like text (force names, masses), whatever its
// spacing.
func isLabel(text string) bool {
	compact := strings.TrimSpace(spaces.ReplaceAllString(text, " "))
	if compact == "" || len([]rune(strings.ReplaceAll(compact, " ", ""))) > 30 || len(strings.Fields(compact)) > 8 {
		return false
	}
	return !ethiopic.MatchString(compact) && !labelWords.MatchString(compact) && !strings.HasSuffix(compact, ":")
}

func rotate(px, py, cx, cy, deg float64) (float64, float64) {
	if deg == 0 {
		return px, py
	}
	r := deg * math.Pi / 180
	dx, dy := px-cx, py-cy
	return cx + dx*math.Cos(r) - dy*math.Sin(r), cy + dx*math.Sin(r) + dy*math.Cos(r)
}

// arrowhead: a triangle at (x2, y2) pointing away from (x1, y1).
func arrowhead(x1, y1, x2, y2, size float64) string {
	ang := math.Atan2(y2-y1, x2-x1)
	lx, ly := x2-size*math.Cos(ang-0.45), y2-size*math.Sin(ang-0.45)
	rx, ry := x2-size*math.Cos(ang+0.45), y2-size*math.Sin(ang+0.45)
	return fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="#1f2933"/>`, x2, y2, lx, ly, rx, ry)
}

// svgFor draws shapes and labels (EMUs) as one SVG.
func svgFor(shapes []shape, labels []label) string {
	var xs, ys []float64
	for _, s := range shapes {
		xs = append(xs, s.x, s.x+s.w)
		ys = append(ys, s.y, s.y+s.h)
	}
	for _, l := range labels {
		xs = append(xs, l.x, l.x+float64(len([]rune(l.text)))*l.size*0.55*emuPT)
		ys = append(ys, l.y-l.size*emuPT, l.y+l.size*0.3*emuPT)
	}
	if len(xs) == 0 {
		return ""
	}
	minOf := func(v []float64) float64 {
		m := v[0]
		for _, x := range v {
			m = math.Min(m, x)
		}
		return m
	}
	maxOf := func(v []float64) float64 {
		m := v[0]
		for _, x := range v {
			m = math.Max(m, x)
		}
		return m
	}
	x0, y0 := minOf(xs)-padEMU, minOf(ys)-padEMU
	width, height := maxOf(xs)-x0+padEMU, maxOf(ys)-y0+padEMU
	px := func(v float64) float64 { return v / emuPX }

	var b strings.Builder
	for _, s := range shapes {
		x, y, w, h := px(s.x-x0), px(s.y-y0), px(s.w), px(s.h)
		cx, cy := x+w/2, y+h/2
		stroke := px(s.line)
		switch s.geom {
		case "line", "straightConnector1":
			x1, y1, x2, y2 := x, y, x+w, y+h
			if s.flipH {
				x1, x2 = x2, x1
			}
			if s.flipV {
				y1, y2 = y2, y1
			}
			x1, y1 = rotate(x1, y1, cx, cy, s.rot)
			x2, y2 = rotate(x2, y2, cx, cy, s.rot)
			sw := math.Max(stroke, 1.5)
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#1f2933" stroke-width="%.1f"/>`, x1, y1, x2, y2, sw)
			if s.tail {
				b.WriteString(arrowhead(x1, y1, x2, y2, 6+sw*2))
			}
			if s.head {
				b.WriteString(arrowhead(x2, y2, x1, y1, 6+sw*2))
			}
			continue
		case "rightArrow", "leftArrow", "upArrow", "downArrow":
			ends := map[string][4]float64{
				"rightArrow": {x, cy, x + w, cy}, "leftArrow": {x + w, cy, x, cy},
				"upArrow": {cx, y + h, cx, y}, "downArrow": {cx, y, cx, y + h},
			}[s.geom]
			x1, y1 := rotate(ends[0], ends[1], cx, cy, s.rot)
			x2, y2 := rotate(ends[2], ends[3], cx, cy, s.rot)
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#1f2933" stroke-width="2"/>`, x1, y1, x2, y2)
			b.WriteString(arrowhead(x1, y1, x2, y2, 10))
			continue
		}
		attrs := `stroke="none"`
		if s.line > 0 {
			attrs = fmt.Sprintf(`stroke="#1f2933" stroke-width="%.1f"`, math.Max(stroke, 1.2))
		}
		fill := s.fill
		if fill == "" {
			fill = "none"
		}
		rot := ""
		if s.rot != 0 {
			rot = fmt.Sprintf(` transform="rotate(%.1f %.1f %.1f)"`, s.rot, cx, cy)
		}
		switch {
		case s.geom == "ellipse" || s.geom == "flowChartConnector":
			fmt.Fprintf(&b, `<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="%s" %s%s/>`, cx, cy, w/2, h/2, fill, attrs, rot)
		case s.geom == "triangle":
			fmt.Fprintf(&b, `<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="%s" %s%s/>`, cx, y, x+w, y+h, x, y+h, fill, attrs, rot)
		case s.geom == "rtTriangle" && s.flipH:
			fmt.Fprintf(&b, `<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="%s" %s%s/>`, x+w, y, x, y+h, x+w, y+h, fill, attrs, rot)
		case s.geom == "rtTriangle":
			fmt.Fprintf(&b, `<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="%s" %s%s/>`, x, y, x+w, y+h, x, y+h, fill, attrs, rot)
		case s.picture:
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#eef2f6" stroke="#9aa0a6" stroke-dasharray="4 3"/>`, x, y, w, h)
		case s.line > 0 || s.fill != "":
			r := 0.0
			if s.geom == "roundRect" || s.geom == "flowChartAlternateProcess" {
				r = math.Min(w, h) * 0.15
			}
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f" fill="%s" %s%s/>`, x, y, w, h, r, fill, attrs, rot)
		}
		if s.text != "" && !s.picture {
			size := s.size * emuPT / emuPX
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="%.0f" text-anchor="middle" font-family="sans-serif" fill="#1f2933">%s</text>`,
				cx, cy+size*0.35, size, html.EscapeString(s.text))
		}
	}
	for _, l := range labels {
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="%.0f" font-family="sans-serif" fill="#1f2933">%s</text>`,
			px(l.x-x0), px(l.y-y0), l.size*emuPT/emuPX, html.EscapeString(l.text))
	}
	vw, vh := px(width), px(height)
	shown := math.Min(vw, maxSvgW)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f" width="%.0f" height="%.0f">`, vw, vh, shown, vh*shown/vw) +
		b.String() + `</svg>`
}
