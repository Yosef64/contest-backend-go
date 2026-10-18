package docx

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

const (
	docHead = `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
 xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"
 xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
 xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
 xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"
 xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"
 xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>`
	docTail = `</w:body></w:document>`
)

func run(text, props string) string {
	return `<w:r><w:rPr>` + props + `</w:rPr><w:t xml:space="preserve">` + text + `</w:t></w:r>`
}

func para(inner string) string { return `<w:p>` + inner + `</w:p>` }

const body16 = `<w:sz w:val="32"/>`

// A small chapter: two "CHAPTER" lessons, headings by formatting, a list,
// an equation, coloured text, a diagram and a picture.
func sampleDocx(t *testing.T) []byte {
	t.Helper()
	box := `<w:r><w:drawing><wp:anchor><wp:positionH relativeFrom="column"><wp:posOffset>914400</wp:posOffset></wp:positionH>
<wp:positionV relativeFrom="paragraph"><wp:posOffset>0</wp:posOffset></wp:positionV><wp:extent cx="600000" cy="380000"/>
<a:graphic><a:graphicData><wps:wsp><wps:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="600000" cy="380000"/></a:xfrm>
<a:prstGeom prst="rect"/><a:ln w="12700"/></wps:spPr><wps:txbx><w:txbxContent><w:p><w:r><w:t>M</w:t></w:r></w:p></w:txbxContent></wps:txbx></wps:wsp></a:graphicData></a:graphic></wp:anchor></w:drawing></w:r>`
	arrow := `<w:r><w:drawing><wp:anchor><wp:positionH relativeFrom="column"><wp:posOffset>1200000</wp:posOffset></wp:positionH>
<wp:positionV relativeFrom="paragraph"><wp:posOffset>0</wp:posOffset></wp:positionV><wp:extent cx="0" cy="400000"/>
<a:graphic><a:graphicData><wps:wsp><wps:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="400000"/></a:xfrm>
<a:prstGeom prst="straightConnector1"/><a:ln w="19050"><a:tailEnd type="triangle"/></a:ln></wps:spPr></wps:wsp></a:graphicData></a:graphic></wp:anchor></w:drawing></w:r>`
	picture := `<w:r><w:drawing><wp:inline><wp:extent cx="900000" cy="600000"/><a:graphic><a:graphicData><pic:pic><pic:blipFill>
<a:blip r:embed="rIdImg"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
	fraction := `<m:oMath><m:r><m:t>F=</m:t></m:r><m:f><m:num><m:r><m:t>m</m:t></m:r></m:num><m:den><m:r><m:t>a</m:t></m:r></m:den></m:f></m:oMath>`
	doc := docHead +
		para(run("CHAPTER FOUR", `<w:b/><w:sz w:val="44"/>`)) +
		para(run("Dynamics", `<w:b/><w:sz w:val="40"/>`)) +
		para(run("Force is a push or a pull. ", body16)+run("Inertia", `<w:b/><w:color w:val="1F4E79"/>`+body16)+run(" resists it.", body16)) +
		para(run("Newtons 2nd law of motion", `<w:b/><w:sz w:val="44"/>`)) +
		para(`<w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr>`+run("ሃይል ማለት", body16)) +
		para(run("It says ", body16)+fraction+run(" here.", body16)) +
		para(run("1, FORCE OF GRAVITY (weight)", `<w:b/>`+body16)) +
		para(run("Solution", `<w:b/>`+body16)) +
		para(box+arrow) +
		para(run("Fg", body16)) +
		para(run("ይህ ኃይል ወደ ታች ይስባል። Back to text after the diagram.", body16)) +
		para(picture) +
		para(run("VICTORY IS A STATE OF MIND!", `<w:b/>`+body16)) +
		para(run("CHAPTER FIVE", `<w:b/><w:sz w:val="44"/>`)) +
		para(run("Work and Energy", `<w:b/><w:sz w:val="40"/>`)) +
		para(run("1.Work Done", `<w:b/><w:sz w:val="36"/>`)) +
		para(run("Work is force times displacement.", body16)) +
		docTail
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, _ := z.Create(name)
		_, _ = w.Write([]byte(body))
	}
	add("word/document.xml", doc)
	add("word/_rels/document.xml.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rIdImg" Type="image" Target="media/image1.png"/></Relationships>`)
	add("word/media/image1.png", "\x89PNG fake")
	add("word/numbering.xml", `<?xml version="1.0"?><w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:numFmt w:val="bullet"/></w:lvl></w:abstractNum>
<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num></w:numbering>`)
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestConvert(t *testing.T) {
	var saved []string
	res, err := Convert(context.Background(), sampleDocx(t), Options{
		SavePicture: func(_ context.Context, data []byte, name string) (string, error) {
			saved = append(saved, name+":"+string(data[:4]))
			return "https://img.example/" + name, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Lessons) != 2 || res.Lessons[0].Title != "Dynamics" || res.Lessons[1].Title != "Work and Energy" {
		t.Fatalf("lessons: %+v", res.Lessons)
	}
	one, two := res.Lessons[0].HTML, res.Lessons[1].HTML
	for _, want := range []string{
		`<h2>Newtons 2nd law of motion</h2>`,                    // bold 22 pt: a topic
		`<h3>1, FORCE OF GRAVITY (weight)</h3>`,                 // bold numbered: a sub-heading
		`<p><strong>Solution</strong></p>`,                      // a label, not a heading
		`<span style="color: #1F4E79"><strong>Inertia</strong>`, // colour kept
		`<ul>`, `<li>ሃይል ማለት</li>`, // list, Amharic
		`<span data-tex="F=\frac{m}{a}"></span>`,      // inline equation
		`<svg`, `>M</text>`, `>Fg</text>`, `<polygon`, // diagram: box, label, arrowhead
		`Back to text after the diagram.`,           // text after the diagram is text
		`<img src="https://img.example/image1.png"`, // picture stored
		`<div class="callout key">`,                 // motto
	} {
		if !strings.Contains(one, want) {
			t.Errorf("lesson 1 lacks %q:\n%s", want, one)
		}
	}
	if strings.Contains(one, "<h2>Solution") || strings.Contains(one, "<h3>Solution") {
		t.Error("Solution became a heading")
	}
	if !strings.Contains(two, `<h2>1.Work Done</h2>`) {
		t.Errorf("numbered 18 pt title should be a topic:\n%s", two)
	}
	if res.Equations != 1 || res.Diagrams != 1 || res.Pictures != 1 || len(saved) != 1 || saved[0] != "image1.png:\x89PNG" {
		t.Errorf("counts: eq %d diagrams %d pictures %d saved %v", res.Equations, res.Diagrams, res.Pictures, saved)
	}
	if res.Lessons[0].Summary != "Force is a push or a pull. Inertia resists it." {
		t.Errorf("summary %q", res.Lessons[0].Summary)
	}
}

func TestConvertWithoutPictureStore(t *testing.T) {
	res, err := Convert(context.Background(), sampleDocx(t), Options{
		SavePicture: func(context.Context, []byte, string) (string, error) { return "", errors.New("unsupported") },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Lessons[0].HTML, "Picture 1: add it here in the editor.") || len(res.Warnings) != 1 {
		t.Errorf("a picture that can't be stored leaves a note and a warning: %v", res.Warnings)
	}
}

func TestNotDocx(t *testing.T) {
	if _, err := Convert(context.Background(), []byte("hello"), Options{}); !errors.Is(err, ErrNotDocx) {
		t.Fatalf("err = %v", err)
	}
}

func TestOMML(t *testing.T) {
	cases := map[string]string{
		`<m:oMath><m:sSup><m:e><m:r><m:t>x</m:t></m:r></m:e><m:sup><m:r><m:t>2</m:t></m:r></m:sup></m:sSup></m:oMath>`:             `{x}^{2}`,
		`<m:oMath><m:rad><m:radPr><m:degHide m:val="1"/></m:radPr><m:deg/><m:e><m:r><m:t>g</m:t></m:r></m:e></m:rad></m:oMath>`:    `\sqrt{g}`,
		`<m:oMath><m:r><m:t>μ</m:t></m:r><m:r><m:t>mg</m:t></m:r></m:oMath>`:                                                       `\mu mg`,
		`<m:oMath><m:d><m:dPr><m:begChr m:val="{"/><m:endChr m:val=""/></m:dPr><m:e><m:r><m:t>a</m:t></m:r></m:e></m:d></m:oMath>`: `\left\{a\right.`,
		`<m:oMath><m:acc><m:accPr><m:chr m:val="⃗"/></m:accPr><m:e><m:r><m:t>F</m:t></m:r></m:e></m:acc></m:oMath>`:                `\vec{F}`,
		`<m:oMath><m:func><m:fName><m:r><m:t>cos</m:t></m:r></m:fName><m:e><m:r><m:t>θ</m:t></m:r></m:e></m:func></m:oMath>`:       `\cos \theta`,
	}
	for src, want := range cases {
		root, err := parse([]byte(`<w:x xmlns:w="` + nsW + `" xmlns:m="` + nsM + `">` + src + `</w:x>`))
		if err != nil {
			t.Fatal(err)
		}
		if got := tidyTex(omml(root.find("m:oMath"))); got != want {
			t.Errorf("%s\n got %q\nwant %q", src, got, want)
		}
	}
}
