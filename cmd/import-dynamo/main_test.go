package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestPlanTransformsOldData(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	qs := []oldQuestion{
		{ID: "q1", QuestionText: "1+1?", Answer: 2, Grade: "1", Subject: "Mathematics", Chapter: "Ch 1", MultipleChoice: []string{"1", "2", "3", "4"}},
		{ID: "q2", QuestionText: "2+2?", Answer: 1, Grade: "1", Subject: "Mathematics", Chapter: "Ch 1", MultipleChoice: []string{"4", "5"}},
		{ID: "bad", QuestionText: "?", Answer: 9, Grade: "1", MultipleChoice: []string{"a", "b"}},
	}
	contests := []oldContest{{ID: "c1", Title: "Weekly", Grade: "1", Subject: "Mathematics",
		StartTime: "2025-05-01T10:00:00Z", EndTime: "2025-05-01T11:00:00Z", Questions: []string{"q1", "q2", "bad"}}}
	students := []oldStudent{{ID: "s1", TelegramID: "777", Name: "Abebe", Age: "16", Grade: "2"}}
	regs := []oldRegistration{{ContestID: "c1", StudentID: "s1"}, {ContestID: "gone", StudentID: "s1"}}
	subs := []oldSubmission{{ContestID: "c1", StudentID: "s1", Score: 1, TimeSpend: "00:02:05",
		SubmissionTime: time.Now()}}
	subs[0].MissedQuestions = append(subs[0].MissedQuestions, struct {
		ID             string `dynamodbav:"id"`
		SelectedAnswer int    `dynamodbav:"selected_answer"`
	}{ID: "q2", SelectedAnswer: 2})
	admins := []oldAdmin{{ID: "a1", Email: "A@x.com", Password: "pw", IsApproved: true}}

	p := plan(log, admins, students, qs, contests, regs, subs)

	counts := map[string]int{}
	for k, v := range p.rows {
		counts[k] = len(v)
	}
	want := map[string]int{"admins": 1, "students": 1, "questions": 2, "contests": 1, "contest_qs": 2, "registrations": 1, "submissions": 1}
	for k, n := range want {
		if counts[k] != n {
			t.Errorf("%s: got %d rows, want %d", k, counts[k], n)
		}
	}
	if q := p.rows["questions"][0]; q[3] != 1 || q[7] != "9" {
		t.Errorf("question answer/grade not converted: %v", q)
	}
	if s := p.rows["students"][0]; s[0] != "777" || s[1] != "telegram" || s[8] != "10" || s[6] != 16 {
		t.Errorf("student not converted: %v", s)
	}
	if r := p.rows["registrations"][0]; r[1] != "777" {
		t.Errorf("registration student id not remapped: %v", r)
	}
	sub := p.rows["submissions"][0]
	var answers map[string]int
	_ = json.Unmarshal(sub[2].([]byte), &answers)
	if answers["q1"] != 1 || answers["q2"] != 1 || sub[3] != 1 || sub[4] != 2 || sub[5] != 125 {
		t.Errorf("submission not rebuilt: answers=%v row=%v", answers, sub)
	}
	if a := p.rows["admins"][0]; a[2] != "a@x.com" || a[3] == "pw" {
		t.Errorf("admin email/password not normalised: %v", a)
	}
}
