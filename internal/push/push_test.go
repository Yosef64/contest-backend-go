package push

import "testing"

func TestAudience(t *testing.T) {
	school := "a1b2-c3"
	empty := ""
	cases := []struct {
		dept             string
		school           *string
		topic, condition string
	}{
		{"", nil, "all", ""},
		{"", &empty, "all", ""},
		{"remedial", nil, "dept_remedial", ""},
		{"", &school, "school_a1b2-c3", ""},
		{"entrance", &school, "", "'dept_entrance' in topics && 'school_a1b2-c3' in topics"},
	}
	for _, c := range cases {
		topic, condition := Audience(c.dept, c.school)
		if topic != c.topic || condition != c.condition {
			t.Errorf("Audience(%q, %v) = %q, %q; want %q, %q", c.dept, c.school, topic, condition, c.topic, c.condition)
		}
	}
	if got := Topic("student", "g_11/26 x"); got != "student_g_11_26_x" {
		t.Errorf("Topic = %q", got)
	}
}

func TestNewWithoutConfig(t *testing.T) {
	s, err := New("", "", "")
	if s != nil || err != nil {
		t.Fatalf("New() = %v, %v; want nil, nil", s, err)
	}
	// A nil sender sends nothing (push not configured).
	if err := s.Send(t.Context(), Message{Topic: "all"}); err != nil {
		t.Fatal(err)
	}
}
