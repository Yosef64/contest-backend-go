package content

import (
	"strings"
	"testing"
)

// Diagrams drawn in the admin editor must come through the sanitizer whole:
// shapes, their class names (to edit them again), and equations drawn as
// nested groups with their TeX.
func TestDrawnDiagramSurvivesSanitizing(t *testing.T) {
	in := `<figure><svg class="victory-diagram" xmlns="http://www.w3.org/2000/svg" viewBox="10 20 300 200" width="300" height="200">` +
		`<g class="d-arrow"><line x1="20" y1="30" x2="200" y2="30" stroke="#2563EB" stroke-width="2"/><polygon points="200,30 186,24 186,36" fill="#2563EB"/></g>` +
		`<polygon class="d-poly" points="100,340 520,340 520,90" fill="none" stroke="#121715" stroke-width="2" stroke-dasharray="8 6"/>` +
		`<text class="d-text" x="50" y="160" font-size="13" font-family="sans-serif" fill="#121715">30°</text>` +
		`<g class="d-math" data-tex="mg\sin\theta" font-size="17" transform="translate(350 190) scale(0.01955)">` +
		`<g transform="translate(0 705)"><g stroke="#121715" fill="#121715" transform="scale(1,-1)">` +
		`<g><g><path d="M1 2L3 4"></path></g><g transform="translate(878,0)"><path d="M5 6L7 8"></path></g></g>` +
		`</g></g></g></svg></figure>`
	out := SanitizeHTML(in)
	for _, want := range []string{
		`class="victory-diagram"`, `viewBox="10 20 300 200"`, `class="d-arrow"`,
		`class="d-poly"`, `stroke-dasharray="8 6"`, `class="d-text"`, `>30°</text>`,
		`class="d-math"`, `data-tex="mg\sin\theta"`, `transform="translate(350 190) scale(0.01955)"`,
		// The plain groups stay, so "g" isn't nested inside the first letter's group.
		`<g><g><path d="M1 2L3 4"></path></g><g transform="translate(878,0)">`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("lost %s\n%s", want, out)
		}
	}
	if strings.Count(out, "<g") != strings.Count(out, "</g>") {
		t.Errorf("unbalanced groups:\n%s", out)
	}
}
