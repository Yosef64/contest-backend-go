package api

import (
	"testing"
	"time"
)

func TestReportDateRange(t *testing.T) {
	for _, tc := range []struct {
		name, from, to string
		bad            bool
	}{
		{"unfiltered", "", "", false}, {"same day", "2026-10-03", "2026-10-03", false}, {"start only", "2026-10-03", "", false}, {"invalid day", "2026-02-30", "", true}, {"reversed", "2026-10-04", "2026-10-03", true}, {"bad end", "", "not-a-date", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end, err := reportDateRange(tc.from, tc.to)
			if (err != nil) != tc.bad {
				t.Fatalf("unexpected error %v", err)
			}
			if tc.from == tc.to && tc.from != "" && !tc.bad {
				if end.Sub(*start) != 24*time.Hour || end.Location() != time.UTC {
					t.Fatal("calendar end day must be inclusive in UTC")
				}
			}
		})
	}
}
