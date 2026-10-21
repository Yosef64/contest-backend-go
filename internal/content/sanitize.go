// Package content sanitizes chapter HTML before it is stored, so only the
// markup the editor and the app support can reach students.
package content

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var policy = newPolicy()

// styleAttr is the inline styling the editor produces and the app renders:
// alignment, text and background colour, font size and family, and media
// width. Anything else (positioning, urls, expressions) drops the attribute.
var styleAttr = regexp.MustCompile(`^\s*((` + strings.Join([]string{
	`text-align:\s*(left|center|right|justify)`,
	`(background-)?color:\s*(#[0-9a-fA-F]{3,8}|inherit)`,
	`width:\s*\d{1,3}%`,
	`font-size:\s*\d{1,2}(\.\d{1,2})?(px|em|rem)`,
	`font-family:\s*(serif|sans-serif|monospace|cursive)`,
}, "|") + `);?\s*)+$`)

func newPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// Equations: their TeX (an equation on a diagram is a <g> of shapes
	// that keeps its TeX so it can be edited again).
	p.AllowAttrs("data-tex").OnElements("span", "div", "g")
	p.AllowStyling() // class attributes
	p.AllowElements("figure", "figcaption", "mark", "u", "s", "sub", "sup")
	p.AllowAttrs("style").Matching(styleAttr).Globally()
	p.AllowAttrs("data-youtube-video").OnElements("div")

	// Video: our own files or any https source.
	p.AllowElements("video", "source")
	p.AllowAttrs("controls", "width", "height", "preload", "poster").OnElements("video")
	p.AllowAttrs("src").Matching(regexp.MustCompile(`^https?://`)).OnElements("video", "source")
	p.AllowAttrs("type").Matching(regexp.MustCompile(`^video/(mp4|webm)$`)).OnElements("source")

	// YouTube embeds only.
	p.AllowElements("iframe")
	p.AllowAttrs("src").Matching(regexp.MustCompile(`^https://(www\.)?(youtube\.com|youtube-nocookie\.com)/embed/[\w-]+`)).OnElements("iframe")
	p.AllowAttrs("width", "height", "allowfullscreen", "frameborder", "allow").OnElements("iframe")
	// YouTube refuses to play without a referrer (Error 153).
	p.AllowAttrs("referrerpolicy").Matching(regexp.MustCompile(`^strict-origin-when-cross-origin$`)).OnElements("iframe")

	// Inline SVG diagrams: shapes and text only (no scripts, no foreign content).
	svgEls := []string{"svg", "g", "rect", "circle", "ellipse", "line", "polyline", "polygon", "path", "text", "tspan", "defs", "lineargradient", "stop"}
	p.AllowElements(svgEls...)
	// A group with no attributes still groups (equations drawn on diagrams
	// nest them; dropping one moves everything after it).
	p.AllowNoAttrs().OnElements("g")
	p.AllowAttrs(
		"viewbox", "width", "height", "x", "y", "x1", "y1", "x2", "y2", "cx", "cy", "r", "rx", "ry",
		"points", "d", "fill", "stroke", "stroke-width", "stroke-linecap", "stroke-linejoin", "stroke-dasharray",
		"font-size", "font-weight", "font-family", "text-anchor", "transform", "opacity", "offset", "stop-color", "xmlns",
	).OnElements(svgEls...)

	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	return p
}

// The sanitizer lowercases names; restore SVG's case-sensitive ones so
// stored lessons are valid for every renderer.
var svgCase = strings.NewReplacer(
	" viewbox=", " viewBox=",
	"<lineargradient", "<linearGradient",
	"</lineargradient>", "</linearGradient>",
)

// SanitizeHTML strips scripts, event handlers and unsupported elements.
func SanitizeHTML(html string) string {
	return svgCase.Replace(policy.Sanitize(hexColours(html)))
}

var (
	styleAttrs = regexp.MustCompile(`style="[^"]*"`)
	rgbColour  = regexp.MustCompile(`rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*(?:,\s*([\d.]+)\s*)?\)`)
)

// hexColours writes rgb()/rgba() colours in style attributes as #hex. The
// admin editor reads a lesson's #hex colours back from the browser as rgb(),
// so without this every colour was dropped the next time a chapter was
// saved. Text outside style attributes is left alone.
func hexColours(html string) string {
	return styleAttrs.ReplaceAllStringFunc(html, func(attr string) string {
		return rgbColour.ReplaceAllStringFunc(attr, func(c string) string {
			m := rgbColour.FindStringSubmatch(c)
			var b strings.Builder
			b.WriteByte('#')
			for _, v := range m[1:4] {
				n, _ := strconv.Atoi(v)
				fmt.Fprintf(&b, "%02X", min(n, 255))
			}
			if m[4] != "" {
				if a, err := strconv.ParseFloat(m[4], 64); err == nil && a < 1 {
					fmt.Fprintf(&b, "%02X", int(math.Round(max(a, 0)*255)))
				}
			}
			return b.String()
		})
	})
}

// SanitizeText is for fields that are usually plain text but may hold
// formatted HTML (question text, explanations). Plain text is kept as typed —
// clients render it as text, so escaping it would show "&#34;" to students —
// and anything with markup goes through SanitizeHTML.
func SanitizeText(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	return SanitizeHTML(s)
}
