package api

import (
	"testing"

	"victor-contest-go/internal/content"
)

func TestChapaPhone(t *testing.T) {
	for in, want := range map[string]string{
		"0911223344":      "0911223344",
		"+251 911 223344": "0911223344",
		"251711223344":    "0711223344",
		"911223344":       "0911223344",
		"0811223344":      "",
		"12345":           "",
	} {
		if got := chapaPhone(in); got != want {
			t.Errorf("chapaPhone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitName(t *testing.T) {
	if f, l := splitName("Abel Tesfaye Bekele"); f != "Abel" || l != "Tesfaye Bekele" {
		t.Errorf("got %q %q", f, l)
	}
	if f, l := splitName("Hana"); f != "Hana" || l != "Hana" {
		t.Errorf("got %q %q", f, l)
	}
}

func TestPlainText(t *testing.T) {
	got := content.PlainText(`<h2>Cells</h2><p>The cell is the <b>basic</b> unit &amp; life.</p><ul><li>Nucleus</li><li>Ribosome</li></ul><script>x()</script>`)
	want := "Cells\nThe cell is the basic unit & life.\nNucleus\nRibosome"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
