package media

import "testing"

func TestAvatarURLAndLegacyImageID(t *testing.T) {
	s, err := New("cloudinary://key:secret@demo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.AvatarURL("g_123/../x"); got != "https://res.cloudinary.com/demo/image/upload/victory/avatars/g_123____x.jpg" {
		t.Fatalf("avatar url: %s", got)
	}
	cases := map[string]string{
		"https://res.cloudinary.com/demo/image/upload/v1791896045/victory/lessons/abc.png": "victory/lessons/abc",
		"https://res.cloudinary.com/demo/image/upload/victory/lessons/abc.jpg":             "victory/lessons/abc",
		"https://res.cloudinary.com/demo/image/upload/victory/avatars/g_1.jpg":             "",
		"https://res.cloudinary.com/other/image/upload/v1/victory/lessons/abc.png":         "",
		"https://lh3.googleusercontent.com/a/abc=s256-c":                                   "",
		"": "",
	}
	for url, want := range cases {
		if got := s.LegacyImageID(url); got != want {
			t.Errorf("LegacyImageID(%q) = %q, want %q", url, got, want)
		}
	}
}
