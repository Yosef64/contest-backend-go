-- Exam-mode violations: the student unpinned the app or left it during an
-- exam that runs in exam mode. Ordinary phones can't stop that, so the app
-- pauses the exam, reports it here for the proctor, and submits the exam
-- after max_violations (0: never).
ALTER TABLE exam_mode_settings
    ADD COLUMN IF NOT EXISTS max_violations integer NOT NULL DEFAULT 3
        CHECK (max_violations BETWEEN 0 AND 50);

CREATE TABLE IF NOT EXISTS exam_violations (
    id          bigserial PRIMARY KEY,
    contest_id  text NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    student_id  text NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    -- "unpinned", "left_app" or "auto_submitted".
    kind        text NOT NULL CHECK (kind IN ('unpinned', 'left_app', 'auto_submitted')),
    -- When it happened on the phone (reports can arrive late when offline).
    at          timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS exam_violations_contest ON exam_violations (contest_id, student_id, at);
