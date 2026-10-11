package api

import (
	"testing"
	"time"
	"victor-contest-go/internal/store"
)

func TestContestLevelsAndStreams(t *testing.T) {
	for _, grade := range []string{"11", "12", "remedial", "freshman"} {
		t.Run(grade, func(t *testing.T) {
			in := contestInput{Title: "Exam", Subject: "Mathematics", Grade: grade, Stream: "social", StartTime: time.Now(), EndTime: time.Now().Add(time.Hour), QuestionIDs: []string{"q1"}}
			co, err := in.toContest()
			if err != nil {
				t.Fatal(err)
			}
			if co.Grade != grade || co.Stream != "social" {
				t.Fatalf("lost exam audience: %+v", co)
			}
		})
	}
	in := contestInput{Subject: "Mathematics", Grade: "10", Stream: "natural", StartTime: time.Now(), EndTime: time.Now().Add(time.Hour), QuestionIDs: []string{"q1"}}
	co, err := in.toContest()
	if err != nil {
		t.Fatal(err)
	}
	if co.Stream != "" {
		t.Fatal("grade 10 retained an inapplicable stream")
	}
	in.Grade = "unknown"
	if _, err = in.toContest(); err == nil {
		t.Fatal("unsupported level accepted")
	}
}

func TestExamAudienceMatches(t *testing.T) {
	cases := []struct {
		name, target, targetStream, grade, department, stream string
		want                                                  bool
	}{
		{"social grade 11", "11", "social", "11", "", "social", true},
		{"wrong stream", "11", "social", "11", "", "natural", false},
		{"missing stream", "12", "natural", "12", "entrance", "", false},
		{"both streams", "12", "", "12", "entrance", "natural", true},
		{"remedial", "remedial", "natural", "12", "remedial", "natural", true},
		{"freshman", "freshman", "social", "12", "freshman", "social", true},
		{"isolate university levels", "remedial", "", "12", "freshman", "social", false},
		{"isolate entrance", "freshman", "", "12", "entrance", "social", false},
		{"legacy grade 12 remains available", "12", "", "12", "remedial", "social", true},
		{"wrong grade", "11", "", "12", "entrance", "natural", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := examAudienceMatches(store.Contest{Grade: c.target, Stream: c.targetStream}, c.grade, c.department, c.stream); got != c.want {
				t.Fatalf("got %v; want %v", got, c.want)
			}
		})
	}
}
