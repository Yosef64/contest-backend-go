package api

import (
	"hash/fnv"
	"math/rand/v2"

	"victor-contest-go/internal/store"
)

// Each student gets an exam's questions and options in their own order, so
// neighbours can't copy by position. The order comes from a seed of exam,
// student (and question), so it is the same every time the student opens the
// exam, and answers (sent as the position the student saw) map back to the
// original option on submit.

// seededPerm is a permutation of 0..n-1 fixed by parts.
func seededPerm(n int, parts ...string) []int {
	h := fnv.New64a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	seed := h.Sum64()
	return rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15)).Perm(n)
}

// optionOrder is the original option shown at each position, or nil when
// the exam keeps the authored order.
func optionOrder(co store.Contest, studentID string, q store.Question) []int {
	if !co.ShuffleOptions || len(q.Options) < 2 {
		return nil
	}
	return seededPerm(len(q.Options), co.ID, studentID, q.ID)
}

// playQuestions is the exam as this student sees it: no answers, in their
// order.
func playQuestions(co store.Contest, studentID string, qs []store.Question) []store.PublicQuestion {
	order := make([]int, len(qs))
	for i := range order {
		order[i] = i
	}
	if co.ShuffleQuestions && len(qs) > 1 {
		order = seededPerm(len(qs), co.ID, studentID)
	}
	out := make([]store.PublicQuestion, len(qs))
	for i, k := range order {
		q := qs[k]
		p := q.Public()
		if perm := optionOrder(co, studentID, q); perm != nil {
			p.Options = make([]string, len(perm))
			for j, from := range perm {
				p.Options[j] = q.Options[from]
			}
			if len(p.OptionImages) == len(q.Options) {
				p.OptionImages = make([]string, len(perm))
				for j, from := range perm {
					p.OptionImages[j] = q.OptionImages[from]
				}
			}
		}
		out[i] = p
	}
	return out
}

// unshuffleAnswers turns the positions the student chose into original
// option indexes (out-of-range answers are left for scoring to drop).
func unshuffleAnswers(co store.Contest, studentID string, qs []store.Question, given map[string]int) map[string]int {
	out := make(map[string]int, len(given))
	for id, a := range given {
		out[id] = a
	}
	for _, q := range qs {
		a, ok := given[q.ID]
		perm := optionOrder(co, studentID, q)
		if !ok || perm == nil || a < 0 || a >= len(perm) {
			continue
		}
		out[q.ID] = perm[a]
	}
	return out
}
