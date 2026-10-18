package content

import (
	"strings"
	"testing"
)

func TestSanitizeHTML(t *testing.T) {
	cases := []struct {
		name, in      string
		want, notWant []string
	}{
		{
			name:    "strips scripts and handlers",
			in:      `<p onclick="x()">Hi<script>alert(1)</script></p><img src="https://a.test/x.png" onerror="steal()">`,
			want:    []string{"<p>Hi</p>", `src="https://a.test/x.png"`},
			notWant: []string{"script", "onclick", "onerror"},
		},
		{
			name: "keeps lesson classes",
			in:   `<p class="lead">Intro</p><div class="callout tip"><p>Tip</p></div><div class="formula">V = I × R</div>`,
			want: []string{`class="lead"`, `class="callout tip"`, `class="formula"`},
		},
		{
			name:    "keeps svg diagrams with viewBox",
			in:      `<figure><svg viewBox="0 0 10 10" width="10"><rect x="1" y="1" width="5" height="5" rx="2" fill="#fff"/><text x="1" y="9" font-size="3">A</text></svg><figcaption>Cap</figcaption></figure>`,
			want:    []string{"<svg", `viewBox="0 0 10 10"`, "<rect", "<text", "<figcaption>Cap</figcaption>"},
			notWant: []string{"<script"},
		},
		{
			name:    "keeps video and youtube, drops other iframes",
			in:      `<video controls width="640"><source src="https://cdn.test/a.mp4" type="video/mp4"></video><iframe src="https://www.youtube-nocookie.com/embed/abc123"></iframe><iframe src="https://evil.test/x"></iframe>`,
			want:    []string{"<video", `src="https://cdn.test/a.mp4"`, "youtube-nocookie.com/embed/abc123"},
			notWant: []string{"evil.test"},
		},
		{
			name: "svg attributes keep their case",
			in:   `<svg viewBox="0 0 4 4"><rect width="1" height="1"/></svg>`,
			want: []string{`<svg viewBox="0 0 4 4">`},
		},
		{
			name:    "blocks javascript urls",
			in:      `<a href="javascript:alert(1)">x</a><img src="javascript:alert(1)">`,
			notWant: []string{"javascript:"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := SanitizeHTML(tc.in)
			lower := strings.ToLower(out)
			for _, w := range tc.want {
				if !strings.Contains(out, w) && !strings.Contains(lower, strings.ToLower(w)) {
					t.Errorf("missing %q in:\n%s", w, out)
				}
			}
			for _, nw := range tc.notWant {
				if strings.Contains(lower, strings.ToLower(nw)) {
					t.Errorf("unexpected %q in:\n%s", nw, out)
				}
			}
		})
	}
}

func TestSanitizeKeepsMediaWidth(t *testing.T) {
	in := `<img src="https://x.test/a.png" style="width: 60%"><video controls style="width: 50%"><source src="https://x.test/v.mp4" type="video/mp4"></video><div data-youtube-video="" style="width: 75%"></div><p style="width: 10px; position: fixed">x</p>`
	out := SanitizeHTML(in)
	for _, want := range []string{`<img src="https://x.test/a.png" style="width: 60%"`, `<video controls="" style="width: 50%"`, `style="width: 75%"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
	if strings.Contains(out, "position") || strings.Contains(out, "10px") {
		t.Errorf("unsafe style kept: %s", out)
	}
}

func TestSanitizeTextKeepsPlainText(t *testing.T) {
	plain := `Which is "larger": 3 & 4? It's 4`
	if got := SanitizeText(plain); got != plain {
		t.Errorf("plain text changed: %q", got)
	}
	if got := SanitizeText(`Hi <script>alert(1)</script><b>x</b>`); strings.Contains(got, "script") || !strings.Contains(got, "<b>x</b>") {
		t.Errorf("markup not sanitized: %q", got)
	}
}

func TestSanitizeKeepsEditorStyling(t *testing.T) {
	in := `<p><span style="color: #13814f; font-size: 20px; font-family: serif">Big</span>` +
		`<span style="background-color: #fde68a">marked</span>` +
		`<span class="math" data-tex="\frac{a}{b}"></span>` +
		`<span style="position: fixed; color: #000">x</span>` +
		`<span style="font-family: url(evil)">y</span></p>` +
		`<iframe src="https://www.youtube-nocookie.com/embed/abc123" referrerpolicy="strict-origin-when-cross-origin"></iframe>`
	out := SanitizeHTML(in)
	for _, want := range []string{
		`style="color: #13814f; font-size: 20px; font-family: serif"`,
		`style="background-color: #fde68a"`,
		`data-tex="\frac{a}{b}"`,
		`referrerpolicy="strict-origin-when-cross-origin"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
	for _, bad := range []string{"position", "url(evil)"} {
		if strings.Contains(out, bad) {
			t.Errorf("kept %q: %s", bad, out)
		}
	}
}

// The admin editor reads #hex colours back as rgb(); saving must keep them.
func TestSanitizeKeepsEditorColours(t *testing.T) {
	in := `<p><span style="color: rgb(132, 60, 11);"><strong>Coloured</strong></span> ` +
		`<span style="background-color: rgba(253, 230, 138, 0.5)">marked</span> rgb(1, 2, 3) stays text</p>`
	out := SanitizeHTML(in)
	for _, want := range []string{`style="color: #843C0B;"`, `style="background-color: #FDE68A80"`, `rgb(1, 2, 3) stays text`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}
