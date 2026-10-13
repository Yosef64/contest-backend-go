package media

import (
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	if kind, err := Check("video/quicktime", 50<<20); err != nil || kind != "video" {
		t.Fatalf("mov: %q %v", kind, err)
	}
	if _, err := Check("image/png", MaxImageBytes+1); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("big image: %v", err)
	}
	if _, err := Check("text/html", 10); err != ErrUnsupported {
		t.Fatalf("html: %v", err)
	}
}

func TestSignAndDirectURL(t *testing.T) {
	s, err := New("cloudinary://key:secret@demo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.SignDirect("video")
	if err != nil || d == nil {
		t.Fatalf("sign: %v", err)
	}
	if d.UploadURL != "https://api.cloudinary.com/v1_1/demo/video/upload" || d.Fields["folder"] != Folder ||
		d.Fields["signature"] == "" || d.Fields["api_key"] != "key" || d.Fields["allowed_formats"] != "mp4,webm,mov" {
		t.Fatalf("direct: %+v", d)
	}
	if _, ok := d.Fields["api_secret"]; ok {
		t.Fatal("secret leaked")
	}

	ok := "https://res.cloudinary.com/demo/video/upload/v1/victory/lessons/clip.mov"
	if got, valid := s.DirectURL(ok, "video"); !valid || !strings.HasSuffix(got, "/clip.mp4") {
		t.Fatalf("mov url: %q %v", got, valid)
	}
	for _, bad := range []string{
		"https://res.cloudinary.com/other/video/upload/v1/victory/lessons/clip.mp4",
		"https://res.cloudinary.com/demo/video/upload/v1/elsewhere/clip.mp4",
		"https://res.cloudinary.com/demo/image/upload/v1/victory/lessons/a.png",
		"https://evil.example/demo/video/upload/v1/victory/lessons/clip.mp4",
	} {
		if _, valid := s.DirectURL(bad, "video"); valid {
			t.Errorf("accepted %s", bad)
		}
	}

	local, _ := New("", t.TempDir(), "")
	if d, _ := local.SignDirect("image"); d != nil {
		t.Fatal("local storage should upload through the API")
	}
}

func TestFileKey(t *testing.T) {
	cases := map[string]string{
		"https://res.cloudinary.com/demo/video/upload/v17/victory/lessons/hr14ue841buonokkmzd1.mp4": "hr14ue841buonokkmzd1",
		"https://api.example/uploads/02aee2dbda828765381423d5.webp?x=1":                             "02aee2dbda828765381423d5",
		"https://res.cloudinary.com/demo/image/upload/a.png":                                        "",
	}
	for url, want := range cases {
		if got := FileKey(url); got != want {
			t.Errorf("FileKey(%q) = %q, want %q", url, got, want)
		}
	}
}
