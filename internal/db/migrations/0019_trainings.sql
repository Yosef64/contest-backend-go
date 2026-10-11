-- Training programs (study strategies, timetables, time management, ...):
-- standalone lessons with text and video, no chapters. They need the
-- "training" enrollment unless an admin makes one free.
CREATE TABLE trainings (
    id         text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    title      text        NOT NULL,
    summary    text        NOT NULL DEFAULT '',
    category   text        NOT NULL DEFAULT '',
    minutes    integer     NOT NULL DEFAULT 10 CHECK (minutes BETWEEN 1 AND 600),
    html       text        NOT NULL DEFAULT '',
    access     text        NOT NULL DEFAULT 'training' CHECK (access IN ('free', 'training')),
    status     text        NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    position   integer     NOT NULL DEFAULT 0,
    version    integer     NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by text        NOT NULL DEFAULT ''
);
CREATE INDEX trainings_order_idx ON trainings (position, created_at);

CREATE TABLE training_progress (
    student_id   text        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    training_id  text        NOT NULL REFERENCES trainings (id) ON DELETE CASCADE,
    completed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (student_id, training_id)
);
