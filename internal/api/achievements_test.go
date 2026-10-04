package api

import (
	"testing"
	"time"
)

func TestStreaks(t *testing.T) {
	day := func(n int) time.Time { return time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -n) }
	today := day(0)
	cases := []struct {
		name          string
		days          []int // days ago, newest first
		current, best int
	}{
		{"nothing yet", nil, 0, 0},
		{"today only", []int{0}, 1, 1},
		{"yesterday keeps the streak alive", []int{1, 2, 3}, 3, 3},
		{"two days ago breaks it", []int{2, 3}, 0, 2},
		{"gap inside history", []int{0, 1, 5, 6, 7, 8}, 2, 4},
	}
	for _, c := range cases {
		var days []time.Time
		for _, n := range c.days {
			days = append(days, day(n))
		}
		cur, best := streaks(days, today)
		if cur != c.current || best != c.best {
			t.Errorf("%s: got %d/%d, want %d/%d", c.name, cur, best, c.current, c.best)
		}
	}
}
