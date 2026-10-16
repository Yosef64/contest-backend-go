package api

import (
	"errors"
	"strings"

	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/store"
)

// defaultSubjects are the subjects every academy starts with (migration
// 0023); tests check against them.
var defaultSubjects = map[string]bool{"Mathematics": true, "Physics": true, "Chemistry": true, "Biology": true, "English": true, "History": true, "Geography": true, "Economics": true, "Civics": true}

// subjectOK reports whether name is a subject; mixed allows "Mixed" (an
// exam across subjects). The server replaces it with a lookup in the
// subjects table (see NewServer).
var subjectOK = func(name string, mixed bool) bool {
	if name == "Mixed" {
		return mixed
	}
	return defaultSubjects[name]
}

// NormalizeGrade accepts "9".."12", and the old mini app's "1".."4".
func NormalizeGrade(g string) string {
	g = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(g), "grade "))
	switch g {
	case "1":
		return "9"
	case "2":
		return "10"
	case "3":
		return "11"
	case "4":
		return "12"
	case "9", "10", "11", "12":
		return g
	}
	return ""
}

// storeErr maps store errors to API errors with resource-specific wording.
func storeErr(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return httpx.NotFound(what)
	case errors.Is(err, store.ErrDuplicate):
		return httpx.Conflict("duplicate", what+" already exists.")
	case errors.Is(err, store.ErrInUse):
		return httpx.Conflict("in_use", what+" is in use and can't be changed.")
	case errors.Is(err, store.ErrVersionConflict):
		return httpx.Conflict("version_conflict", "Someone else saved a newer version of this chapter.")
	}
	return err
}

func trimAll(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
