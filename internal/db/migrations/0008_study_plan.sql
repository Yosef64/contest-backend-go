-- Study plan: sessions a student schedules on their calendar.
CREATE TABLE study_sessions (
    id         text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    student_id text        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    title      text        NOT NULL,
    subject    text        NOT NULL DEFAULT '',
    starts_at  timestamptz NOT NULL,
    minutes    integer     NOT NULL CHECK (minutes BETWEEN 5 AND 600),
    notes      text        NOT NULL DEFAULT '',
    done       boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX study_sessions_student_idx ON study_sessions (student_id, starts_at);
