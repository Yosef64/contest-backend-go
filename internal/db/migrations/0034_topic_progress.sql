-- The topics of a chapter a student has finished (chapters are read one
-- topic at a time), so progress follows the account to another phone.
CREATE TABLE IF NOT EXISTS topic_progress (
    student_id text NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    chapter_id text NOT NULL REFERENCES chapters(id) ON DELETE CASCADE,
    -- Finished topic titles.
    topics     text[] NOT NULL DEFAULT '{}',
    -- How many topics the chapter had when last read.
    total      integer NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (student_id, chapter_id)
);
