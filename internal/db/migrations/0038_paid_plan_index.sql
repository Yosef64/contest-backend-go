-- "Does this student have an approved, unexpired plan?" runs on almost every
-- request (course locks, exams, challenges, reports): answer it from an index
-- instead of scanning every enrollment request.
CREATE INDEX IF NOT EXISTS enrollment_approved
    ON enrollment_requests (student_id, module, expires_at)
    WHERE status = 'approved';
