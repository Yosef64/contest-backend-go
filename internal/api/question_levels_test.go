package api

import "testing"

func TestQuestionLevelsAndStreams(t *testing.T) {
	zero := 0
	base := questionInput{Text: "2 + 2?", Options: []string{"3", "4"}, AnswerIndex: &zero, Subject: "Mathematics", Chapter: "Numbers"}
	cases := []struct {
		grade, stream, wantGrade, wantStream string
		ok                                   bool
	}{
		{"9", "natural", "9", "", true}, // grades 9 and 10 have no stream
		{"Grade 10", "", "10", "", true},
		{"11", "natural", "11", "natural", true},
		{"12", "", "12", "", true}, // both streams
		{"Freshman", "Social", "freshman", "social", true},
		{"remedial", "natural", "remedial", "natural", true},
		{"12", "science", "", "", false},
		{"university", "", "", "", false},
	}
	for _, c := range cases {
		in := base
		in.Grade, in.Stream = c.grade, c.stream
		q, err := in.toQuestion("")
		if (err == nil) != c.ok {
			t.Fatalf("%s/%s: err = %v", c.grade, c.stream, err)
		}
		if c.ok && (q.Grade != c.wantGrade || q.Stream != c.wantStream) {
			t.Fatalf("%s/%s: got %s/%s", c.grade, c.stream, q.Grade, q.Stream)
		}
	}
}
