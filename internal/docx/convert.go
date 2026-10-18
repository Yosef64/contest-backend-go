package docx

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Lesson is one chapter of the document, ready to import.
type Lesson struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Minutes int    `json:"minutes"`
	HTML    string `json:"html"`
}

// Result is the converted document and what was found in it.
type Result struct {
	Lessons   []Lesson `json:"chapters"`
	Equations int      `json:"equations"`
	Diagrams  int      `json:"diagrams"`
	Pictures  int      `json:"pictures"`
	// Outline: chapter titles, topics (indented) and sub-headings (more).
	Outline  []string `json:"outline"`
	Warnings []string `json:"warnings"`
}

// Options: Title names the first lesson instead of its "CHAPTER" title;
// SavePicture stores a picture and returns its URL (nil keeps a note in its
// place to add it in the editor).
type Options struct {
	Title       string
	SavePicture func(ctx context.Context, data []byte, name string) (string, error)
}

// ErrNotDocx is returned for files that are not Word documents.
var ErrNotDocx = errors.New("this is not a Word document (.docx)")

const maxXML = 64 << 20 // decompressed document size cap (zip bombs)

var (
	ethiopic   = regexp.MustCompile(`[\x{1200}-\x{137F}]`)
	labelWords = regexp.MustCompile(`(?i)^(solution|given|required|answer|example|examples|note|therefore|so)\b`)
	chapterRe  = regexp.MustCompile(`(?i)^chapter\b`)
	numbered   = regexp.MustCompile(`^\d+\s*[.)]\s*\S`)
	numberedH3 = regexp.MustCompile(`^\d+\s*[,.)]\s*[A-Z]`)
	motto      = regexp.MustCompile(`^[A-Z][A-Z \-]{6,}!$`)
	hexColour  = regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)
	tags       = regexp.MustCompile(`<[^>]+>`)
	marker     = regexp.MustCompile("\x00(INLINE|DISPLAY)(\\d+)\x00")
	emptyP     = regexp.MustCompile(`<p>\s*</p>`)
)

type docFile struct {
	zip   *zip.Reader
	files map[string]*zip.File
}

func (d docFile) read(name string) (*node, error) {
	f, ok := d.files[name]
	if !ok {
		return nil, nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxXML))
	if err != nil {
		return nil, err
	}
	return parse(data)
}

func (d docFile) bytes(name string) ([]byte, error) {
	f, ok := d.files[name]
	if !ok {
		return nil, fmt.Errorf("missing %s", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxXML))
}

// Convert reads a .docx and returns its lessons.
func Convert(ctx context.Context, data []byte, opt Options) (Result, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Result{}, ErrNotDocx
	}
	d := docFile{zip: zr, files: map[string]*zip.File{}}
	for _, f := range zr.File {
		d.files[f.Name] = f
	}
	doc, err := d.read("word/document.xml")
	if err != nil || doc == nil {
		return Result{}, ErrNotDocx
	}
	body := doc.find("w:body")
	if body == nil {
		return Result{}, ErrNotDocx
	}
	styles, _ := d.read("word/styles.xml")
	settings, _ := d.read("word/settings.xml")
	numbering, _ := d.read("word/numbering.xml")
	relsDoc, _ := d.read("word/_rels/document.xml.rels")
	rels := map[string]string{}
	if relsDoc != nil {
		// <Relationships><Relationship Id Target/>...</Relationships>
		for _, k := range relsDoc.kids {
			for _, r := range k.kids {
				rels[r.attr("Id")] = r.attr("Target")
			}
		}
	}
	c := &converter{
		ctx: ctx, opt: opt, doc: d, rels: rels,
		kinds: listKinds(numbering), layout: newLayout(styles, settings), bodySize: bodySize(body),
		title: opt.Title,
	}
	c.run(body)
	return c.result(), nil
}

type converter struct {
	ctx      context.Context
	opt      Options
	doc      docFile
	rels     map[string]string
	kinds    map[string]string
	layout   *layout
	bodySize int

	res       Result
	lessons   [][2]string // title, html
	blocks    []string
	title     string
	wantTitle bool
	listOpen  string

	diagram []shape
	labels  []label
	diagY   float64
}

func (c *converter) flushDiagram() {
	if len(c.diagram) > 0 {
		if svg := svgFor(c.diagram, c.labels); svg != "" {
			c.res.Diagrams++
			c.blocks = append(c.blocks, fmt.Sprintf(`<figure>%s<figcaption>Diagram %d</figcaption></figure>`, svg, c.res.Diagrams))
		}
	}
	c.diagram, c.labels, c.diagY = nil, nil, 0
}

func (c *converter) closeList() {
	if c.listOpen != "" {
		c.blocks = append(c.blocks, "</"+c.listOpen+">")
		c.listOpen = ""
	}
}

func (c *converter) endLesson() {
	c.flushDiagram()
	c.closeList()
	if len(c.blocks) > 0 {
		c.lessons = append(c.lessons, [2]string{c.title, strings.Join(c.blocks, "\n")})
	}
	c.blocks = nil
}

func (c *converter) run(body *node) {
	var items []*node
	for _, k := range body.kids {
		if k.is("w:p") || k.is("w:tbl") {
			items = append(items, k)
		}
	}
	for i, el := range items {
		if el.is("w:tbl") {
			c.flushDiagram()
			c.closeList()
			c.table(el)
			continue
		}
		c.paragraph(el, items, i)
	}
	c.endLesson()
}

func (c *converter) table(el *node) {
	var rows []string
	for _, tr := range el.children("w:tr") {
		var cells []string
		for _, tc := range tr.children("w:tc") {
			var inner []string
			for _, p := range tc.children("w:p") {
				h, _, maths := paraContent(p)
				c.res.Equations += len(maths)
				if r := renderInline(h, maths); strings.TrimSpace(r) != "" {
					inner = append(inner, r)
				}
			}
			cells = append(cells, "<td>"+strings.Join(inner, "<br>")+"</td>")
		}
		rows = append(rows, "<tr>"+strings.Join(cells, "")+"</tr>")
	}
	c.blocks = append(c.blocks, "<table>"+strings.Join(rows, "")+"</table>")
}

func hasAnchor(p *node) bool { return p != nil && p.is("w:p") && p.find("wp:anchor") != nil }

func (c *converter) paragraph(p *node, items []*node, idx int) {
	h, plain, maths := paraContent(p)
	text := strings.TrimSpace(spaces.ReplaceAllString(plain, " "))

	// Diagram paragraphs: shapes, and the short labels typed between them.
	if hasAnchor(p) {
		c.diagram = append(c.diagram, anchoredShapes(p, c.diagY)...)
		var raw strings.Builder
		for _, r := range p.children("w:r") {
			if !skipRun(r) {
				raw.WriteString(r.texts("w:t"))
			}
		}
		if isLabel(raw.String()) && len(maths) == 0 {
			c.labels = append(c.labels, labelTokens(p, c.diagY, c.layout)...)
			c.diagY += c.layout.height(p)
			return
		}
		c.diagY += c.layout.height(p)
		if text == "" && len(maths) == 0 {
			return
		}
	} else if len(c.diagram) > 0 {
		short := isLabel(text) && len(maths) == 0
		more := false
		for _, j := range []int{idx + 1, idx + 2} {
			if j < len(items) && hasAnchor(items[j]) {
				more = true
			}
		}
		if short || (text == "" && len(maths) == 0 && more) {
			if short {
				c.labels = append(c.labels, labelTokens(p, c.diagY, c.layout)...)
			}
			c.diagY += c.layout.height(p)
			return
		}
		c.flushDiagram()
	}

	// Pictures (photos, scans): stored, and shown where they were.
	for _, inl := range p.all("wp:inline") {
		if blip := inl.find("a:blip"); blip != nil {
			c.picture(blip)
		}
	}

	if text == "" && len(maths) == 0 {
		return
	}
	c.res.Equations += len(maths)
	for _, tex := range maths {
		bare := strings.NewReplacer(`\{`, "", `\}`, "").Replace(tex)
		if strings.Count(bare, "{") != strings.Count(bare, "}") {
			c.res.Warnings = append(c.res.Warnings, "An equation may not show correctly: "+tex)
		}
	}

	// "CHAPTER ..." starts a lesson; the next line is its title.
	if chapterRe.MatchString(text) && len([]rune(text)) < 40 {
		c.endLesson()
		c.title = ""
		c.wantTitle = true
		c.res.Outline = append(c.res.Outline, text)
		return
	}
	if c.wantTitle && len([]rune(text)) < 80 && len(maths) == 0 {
		c.title = text
		if c.opt.Title != "" && len(c.lessons) == 0 {
			c.title = c.opt.Title
		}
		c.wantTitle = false
		c.res.Outline = append(c.res.Outline, "  = "+text)
		return
	}

	num := p.child("w:pPr/w:numPr")
	style := styleOf(p)
	level := headingLevel(p, text, len(maths) > 0, c.bodySize)
	if strings.HasPrefix(strings.ToLower(style), "heading") {
		level = 3
		if strings.HasSuffix(style, "1") || strings.HasSuffix(style, "2") {
			level = 2
		}
	}
	bodyHTML := strings.TrimSpace(renderInline(h, maths))
	if level > 0 {
		c.closeList()
		indent := "    "
		if level == 3 {
			indent = "      "
		}
		c.res.Outline = append(c.res.Outline, indent+text)
		c.blocks = append(c.blocks, fmt.Sprintf("<h%d>%s</h%d>", level, html.EscapeString(text), level))
		return
	}
	if motto.MatchString(text) {
		c.closeList()
		c.blocks = append(c.blocks, `<div class="callout key"><p><strong>`+html.EscapeString(text)+`</strong></p></div>`)
		return
	}
	onlyMath := len(maths) > 0 && strings.TrimSpace(marker.ReplaceAllString(h, "")) == ""
	if num != nil && !onlyMath {
		kind := c.kinds[num.val("w:numId")]
		if kind == "" {
			kind = "ul"
		}
		if c.listOpen != kind {
			c.closeList()
			c.blocks = append(c.blocks, "<"+kind+">")
			c.listOpen = kind
		}
		c.blocks = append(c.blocks, "<li>"+bodyHTML+"</li>")
		return
	}
	c.closeList()
	switch {
	case onlyMath && len(maths) == 1:
		c.blocks = append(c.blocks, `<div data-tex="`+html.EscapeString(maths[0])+`"></div>`)
	case onlyMath:
		// Several equations side by side on one line (worked steps).
		var parts []string
		for _, t := range maths {
			parts = append(parts, `<span data-tex="`+html.EscapeString(t)+`"></span>`)
		}
		c.blocks = append(c.blocks, "<p>"+strings.Join(parts, " &nbsp;&nbsp; ")+"</p>")
	default:
		c.blocks = append(c.blocks, emptyP.ReplaceAllString("<p>"+bodyHTML+"</p>", ""))
	}
}

func (c *converter) picture(blip *node) {
	c.res.Pictures++
	n := c.res.Pictures
	target := c.rels[blip.attr("r:embed")]
	if c.opt.SavePicture != nil && target != "" {
		name := path.Clean("word/" + strings.TrimPrefix(strings.TrimPrefix(target, "/"), "word/"))
		if data, err := c.doc.bytes(name); err == nil {
			if url, err := c.opt.SavePicture(c.ctx, data, path.Base(name)); err == nil {
				c.blocks = append(c.blocks, fmt.Sprintf(`<figure><img src="%s" alt="Picture %d"></figure>`, html.EscapeString(url), n))
				return
			} else {
				c.res.Warnings = append(c.res.Warnings, fmt.Sprintf("Picture %d could not be saved (%v); add it in the editor.", n, err))
			}
		}
	}
	c.blocks = append(c.blocks, fmt.Sprintf(`<figure><p class="caption">Picture %d: add it here in the editor.</p></figure>`, n))
}

func (c *converter) result() Result {
	r := c.res
	r.Lessons = []Lesson{}
	for i, l := range c.lessons {
		title, page := l[0], emptyP.ReplaceAllString(l[1], "")
		if title == "" {
			title = fmt.Sprintf("Chapter %d", i+1)
		}
		words := len(strings.Fields(tags.ReplaceAllString(page, " ")))
		minutes := words / 100
		if minutes < 5 {
			minutes = 5
		}
		if minutes > 240 {
			minutes = 240
		}
		first := page
		if i := strings.Index(page, "</p>"); i >= 0 {
			first = page[:i]
		}
		summary := strings.TrimSpace(spaces.ReplaceAllString(html.UnescapeString(tags.ReplaceAllString(first, " ")), " "))
		if rs := []rune(summary); len(rs) > 200 {
			summary = string(rs[:200])
		}
		if n := len([]rune(page)); n > 200000 {
			r.Warnings = append(r.Warnings, fmt.Sprintf(
				"%q is too long to import as one chapter (%d characters, 200,000 at most). Split it into two chapters in Word with a CHAPTER line.", title, n))
		}
		r.Lessons = append(r.Lessons, Lesson{Title: title, Summary: summary, Minutes: minutes, HTML: page})
	}
	if r.Outline == nil {
		r.Outline = []string{}
	}
	if r.Warnings == nil {
		r.Warnings = []string{}
	}
	return r
}

// --- Paragraph text --------------------------------------------------------------

var highlights = map[string]string{
	"yellow": "#FFFF00", "green": "#00FF00", "cyan": "#00FFFF", "magenta": "#FF00FF",
	"blue": "#0000FF", "red": "#FF0000", "darkBlue": "#000080", "darkCyan": "#008080",
	"darkGreen": "#008000", "darkMagenta": "#800080", "darkRed": "#800000",
	"darkYellow": "#808000", "darkGray": "#808080", "lightGray": "#C0C0C0", "black": "#000000",
}

// runColours is the inline style for a run's text colour and highlight;
// black and "automatic" text keep the app's own colours (and dark mode).
func runColours(rpr *node) string {
	var parts []string
	if v := rpr.attrOf("w:color", "w:val"); hexColour.MatchString(v) {
		if u := strings.ToUpper(v); u != "000000" && u != "FFFFFF" {
			parts = append(parts, "color: #"+u)
		}
	}
	back := ""
	if hl := rpr.attrOf("w:highlight", "w:val"); highlights[hl] != "" {
		back = highlights[hl]
	} else if fill := rpr.attrOf("w:shd", "w:fill"); hexColour.MatchString(fill) && strings.ToUpper(fill) != "FFFFFF" {
		back = "#" + strings.ToUpper(fill)
	}
	if back != "" {
		parts = append(parts, "background-color: "+back)
	}
	return strings.Join(parts, "; ")
}

func (n *node) attrOf(child, attr string) string {
	return n.child(child).attr(attr)
}

func on(rpr *node, name string) bool {
	el := rpr.child(name)
	if el == nil {
		return false
	}
	v := el.attr("w:val")
	return v != "0" && v != "false" && v != "none"
}

func runHTML(r *node) string {
	var b strings.Builder
	for _, k := range r.kids {
		switch {
		case k.is("w:t"):
			b.WriteString(html.EscapeString(k.text))
		case k.is("w:tab"):
			b.WriteByte(' ')
		case k.is("w:br"), k.is("w:cr"):
			b.WriteString("<br>")
		case k.is("w:sym"):
			if code, err := strconv.ParseInt(k.attr("w:char"), 16, 32); err == nil {
				if code >= 0xF000 {
					code -= 0xF000
				}
				b.WriteString(html.EscapeString(string(rune(code))))
			}
		}
	}
	text := b.String()
	if strings.TrimSpace(text) == "" {
		return text
	}
	rpr := r.child("w:rPr")
	if rpr == nil {
		return text
	}
	switch rpr.val("w:vertAlign") {
	case "superscript":
		text = "<sup>" + text + "</sup>"
	case "subscript":
		text = "<sub>" + text + "</sub>"
	}
	if on(rpr, "w:b") {
		text = "<strong>" + text + "</strong>"
	}
	if on(rpr, "w:i") {
		text = "<em>" + text + "</em>"
	}
	if on(rpr, "w:u") {
		text = "<u>" + text + "</u>"
	}
	if style := runColours(rpr); style != "" {
		text = `<span style="` + style + `">` + text + `</span>`
	}
	return text
}

// paraContent is a paragraph's HTML (with equation markers), its plain text
// and its equations, without the text of shapes.
func paraContent(p *node) (string, string, []string) {
	var out, plain strings.Builder
	var maths []string
	var walk func(el *node)
	walk = func(el *node) {
		for _, k := range el.kids {
			switch {
			case k.is("w:r"):
				if skipRun(k) {
					continue
				}
				out.WriteString(runHTML(k))
				plain.WriteString(k.texts("w:t"))
			case k.is("m:oMathPara"):
				for _, om := range k.all("m:oMath") {
					if tex := tidyTex(omml(om)); tex != "" {
						maths = append(maths, tex)
						fmt.Fprintf(&out, "\x00DISPLAY%d\x00", len(maths)-1)
					}
				}
			case k.is("m:oMath"):
				if tex := tidyTex(omml(k)); tex != "" {
					maths = append(maths, tex)
					fmt.Fprintf(&out, "\x00INLINE%d\x00", len(maths)-1)
					plain.WriteString(" ⟦m⟧ ")
				}
			case k.is("w:hyperlink"), k.is("w:smartTag"), k.is("w:ins"), k.is("w:sdt"), k.is("w:sdtContent"), k.is("w:fldSimple"):
				walk(k)
			}
		}
	}
	walk(p)
	return out.String(), plain.String(), maths
}

func renderInline(h string, maths []string) string {
	return marker.ReplaceAllStringFunc(h, func(m string) string {
		parts := marker.FindStringSubmatch(m)
		i, _ := strconv.Atoi(parts[2])
		tex := html.EscapeString(maths[i])
		if parts[1] == "DISPLAY" {
			return `</p><div data-tex="` + tex + `"></div><p>`
		}
		return `<span data-tex="` + tex + `"></span>`
	})
}

func listKinds(numbering *node) map[string]string {
	kinds := map[string]string{}
	if numbering == nil {
		return kinds
	}
	abstract := map[string]string{}
	for _, an := range numbering.all("w:abstractNum") {
		kind := "ul"
		if lvl := an.child("w:lvl"); lvl != nil {
			if f := lvl.val("w:numFmt"); f != "" && f != "bullet" && f != "none" {
				kind = "ol"
			}
		}
		abstract[an.attr("w:abstractNumId")] = kind
	}
	for _, num := range numbering.all("w:num") {
		kind := abstract[num.val("w:abstractNumId")]
		if kind == "" {
			kind = "ul"
		}
		kinds[num.attr("w:numId")] = kind
	}
	return kinds
}

// --- Headings, by how the author formatted them ----------------------------------

// paraFormat: all text bold, largest size (half-points), all underlined.
func paraFormat(p *node) (bool, int, bool) {
	bold, underline, seen := true, true, false
	size := 0
	for _, r := range p.children("w:r") {
		if skipRun(r) || strings.TrimSpace(r.texts("w:t")) == "" {
			continue
		}
		seen = true
		rpr := r.child("w:rPr")
		bold = bold && on(rpr, "w:b")
		underline = underline && on(rpr, "w:u")
		if sz := rpr.child("w:sz"); sz != nil {
			if v := atoi(sz.attr("w:val"), 0); v > size {
				size = v
			}
		}
	}
	return bold && seen, size, underline && seen
}

// headingLevel: 2 for a topic (bold, 20 pt and up over a 16 pt body, or a
// numbered "1. Work Done" at 18 pt), 3 for a sub-heading, 0 for text.
func headingLevel(p *node, text string, hasMath bool, body int) int {
	t := strings.TrimSpace(text)
	if t == "" || hasMath || len([]rune(t)) > 100 || labelWords.MatchString(t) ||
		strings.HasSuffix(t, ".") || strings.HasSuffix(t, "!") || strings.HasSuffix(t, ":") ||
		strings.Contains(t, "=") || strings.HasPrefix(t, "=>") || strings.HasPrefix(t, "-") || strings.HasPrefix(t, "*") {
		return 0
	}
	bold, size, underline := paraFormat(p)
	if !bold {
		return 0
	}
	if size >= body+8 || (numbered.MatchString(t) && size >= body+4) {
		return 2
	}
	if size >= body+4 || underline || numberedH3.MatchString(t) {
		return 3
	}
	return 0
}

// bodySize is the most common text size (half-points): the body text.
func bodySize(body *node) int {
	counts := map[int]int{}
	for _, r := range body.all("w:r") {
		sz := r.child("w:rPr/w:sz")
		if n := len([]rune(r.texts("w:t"))); sz != nil && n > 0 {
			counts[atoi(sz.attr("w:val"), 0)] += n
		}
	}
	best, most := 22, 0
	for size, n := range counts {
		if n > most || (n == most && size < best) {
			best, most = size, n
		}
	}
	return best
}
