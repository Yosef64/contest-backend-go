-- Group exams: a student picks the content and duration, friends join a
-- waiting room with an invitation code, and the creator starts the exam for
-- everyone at once.
CREATE TABLE IF NOT EXISTS group_exams (
    id               text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    code             text NOT NULL UNIQUE,
    creator_id       text NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    title            text NOT NULL,
    subject          text NOT NULL DEFAULT '',
    grade            text NOT NULL DEFAULT '',
    -- Where the questions came from: a past national exam set or the bank.
    source_contest_id text REFERENCES contests (id) ON DELETE SET NULL,
    question_ids     text[] NOT NULL,
    duration_minutes integer NOT NULL CHECK (duration_minutes BETWEEN 1 AND 300),
    status           text NOT NULL DEFAULT 'waiting'
                     CHECK (status IN ('waiting', 'running', 'finished', 'cancelled')),
    created_at       timestamptz NOT NULL DEFAULT now(),
    started_at       timestamptz,
    ends_at          timestamptz
);
CREATE INDEX IF NOT EXISTS group_exams_creator_idx ON group_exams (creator_id, created_at DESC);

CREATE TABLE IF NOT EXISTS group_exam_members (
    group_id     text NOT NULL REFERENCES group_exams (id) ON DELETE CASCADE,
    student_id   text NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    joined_at    timestamptz NOT NULL DEFAULT now(),
    -- Answers are saved as the student goes, so a dropped connection or a
    -- closed app loses nothing; the last saved answers count at the end.
    answers      jsonb NOT NULL DEFAULT '{}',
    saved_at     timestamptz,
    submitted_at timestamptz,
    correct      integer,
    total        integer,
    time_seconds integer,
    PRIMARY KEY (group_id, student_id)
);
CREATE INDEX IF NOT EXISTS group_exam_members_student_idx ON group_exam_members (student_id, joined_at DESC);
