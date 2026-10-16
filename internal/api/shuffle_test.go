package api

import (
	"slices"
	"testing"

	"victor-contest-go/internal/store"
)

func TestShuffleIsPerStudentAndMapsAnswersBack(t *testing.T) {
	qs := []store.Question{}
	for _, id := range []string{"q1", "q2", "q3", "q4", "q5", "q6"} {
		qs = append(qs, store.Question{ID: id, Text: id, Options: []string{"A", "B", "C", "D"}, AnswerIndex: 2,
			OptionImages: []string{"", "", "https://img/c.png", ""}})
	}
	co := store.Contest{ID: "exam", ShuffleQuestions: true, ShuffleOptions: true}

	a1, a2 := playQuestions(co, "s1", qs), playQuestions(co, "s1", qs)
	if !slices.EqualFunc(a1, a2, func(x, y store.PublicQuestion) bool { return x.ID == y.ID && slices.Equal(x.Options, y.Options) }) {
		t.Fatal("the same student must always get the same order")
	}
	ids := func(ps []store.PublicQuestion) []string {
		out := []string{}
		for _, p := range ps {
			out = append(out, p.ID+p.Options[0])
		}
		return out
	}
	if slices.Equal(ids(a1), ids(playQuestions(co, "s2", qs))) && slices.Equal(ids(a1), ids(playQuestions(co, "s3", qs))) {
		t.Fatal("different students should see different orders")
	}

	// Choosing the option showing "C" (the answer) scores, wherever it is.
	given := map[string]int{}
	for _, p := range a1 {
		pos := slices.Index(p.Options, "C")
		if p.OptionImages[pos] != "https://img/c.png" {
			t.Fatal("pictures must move with their options")
		}
		given[p.ID] = pos
	}
	_, correct := scoreAnswers(qs, unshuffleAnswers(co, "s1", qs, given))
	if correct != len(qs) {
		t.Fatalf("correct = %d, want %d", correct, len(qs))
	}

	// Shuffling off: authored order, answers untouched.
	off := store.Contest{ID: "exam"}
	if got := playQuestions(off, "s1", qs); got[0].ID != "q1" || got[0].Options[2] != "C" {
		t.Fatal("unshuffled exams keep the authored order")
	}
	if unshuffleAnswers(off, "s1", qs, map[string]int{"q1": 2})["q1"] != 2 {
		t.Fatal("unshuffled answers are kept")
	}
}

func TestOptionsWithImages(t *testing.T) {
	if _, _, ok := optionsWithImages([]string{"A", ""}, nil); ok {
		t.Fatal("an empty option without a picture is invalid")
	}
	opts, pics, ok := optionsWithImages([]string{" A ", ""}, []string{"", "https://x/b.png"})
	if !ok || opts[0] != "A" || pics[1] != "https://x/b.png" {
		t.Fatalf("picture-only option: %v %v %v", opts, pics, ok)
	}
	if _, pics, _ := optionsWithImages([]string{"A", "B"}, []string{"", ""}); len(pics) != 0 {
		t.Fatal("no pictures is stored as an empty list")
	}
}
