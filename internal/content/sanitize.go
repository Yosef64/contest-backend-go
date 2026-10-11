// Package content sanitizes chapter HTML before it is stored, so only the
// markup the editor and the app support can reach students.
package content

import (
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var policy = newPolicy()

func newPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("data-tex").OnElements("span", "div")
	p.AllowStyling() // class attributes
	p.AllowElements("figure", "figcaption", "mark", "u", "s", "sub", "sup")
	p.AllowAttrs("style").Matching(regexp.MustCompile(`^(text-align:\s*(left|center|right|justify);?\s*|color:\s*#[0-9a-fA-F]{3,8};?\s*|width:\s*\d{1,3}%;?\s*)+$`)).Globally()
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

	// Inline SVG diagrams: shapes and text only (no scripts, no foreign content).
	svgEls := []string{"svg", "g", "rect", "circle", "ellipse", "line", "polyline", "polygon", "path", "text", "tspan", "defs", "lineargradient", "stop"}
	p.AllowElements(svgEls...)
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
	return svgCase.Replace(policy.Sanitize(html))
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

