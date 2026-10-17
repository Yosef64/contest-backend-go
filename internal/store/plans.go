package store

// Free and paid results are kept apart (rankings, standings, reports): a
// result is paid when the student had an approved academic plan at the time
// they submitted it. A student who upgrades starts on the paid board with
// their first paid result; their free results stay free.

// PaidAt is an SQL condition: the student had an approved academic plan at
// the given time.
func PaidAt(student, at string) string {
	return "EXISTS (SELECT 1 FROM enrollment_requests e WHERE e.student_id=" + student +
		" AND e.module='academic' AND e.status='approved' AND e.created_at<=" + at + " AND e.expires_at>" + at + ")"
}

// PaidNow is an SQL condition: the student has an approved academic plan now.
func PaidNow(student string) string { return PaidAt(student, "now()") }

// planFilter keeps "paid" or "free" results (anything else: both) of the
// submission alias.
func planFilter(sub, plan string) string {
	switch plan {
	case "paid":
		return " AND " + PaidAt(sub+".student_id", sub+".submitted_at")
	case "free":
		return " AND NOT " + PaidAt(sub+".student_id", sub+".submitted_at")
	}
	return ""
}
