//go:build docxsample

package docx

// Converts a real chapter (not in the repo) and writes the result next to
// it, to compare with scripts/docx_to_lesson.py:
//
//	DOCX_SAMPLE=chapter.docx go test -tags docxsample -run Sample -v ./internal/docx

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSample(t *testing.T) {
	path := os.Getenv("DOCX_SAMPLE")
	if path == "" {
		t.Skip("DOCX_SAMPLE not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Convert(context.Background(), data, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range res.Lessons {
		t.Logf("lesson %q: %d min, %d topics, %d KB", l.Title, l.Minutes, strings.Count(l.HTML, "<h2>"), len(l.HTML)/1024)
	}
	t.Logf("equations %d, diagrams %d, pictures %d, warnings %v", res.Equations, res.Diagrams, res.Pictures, res.Warnings)
	t.Logf("outline:\n%s", strings.Join(res.Outline, "\n"))
	out, _ := json.MarshalIndent(res, "", " ")
	if err := os.WriteFile(strings.TrimSuffix(path, ".docx")+".go.json", out, 0o644); err != nil {
		t.Fatal(err)
	}
}
