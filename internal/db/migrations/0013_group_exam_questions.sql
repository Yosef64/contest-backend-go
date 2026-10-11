-- Group exam questions are written by the student who creates the group
-- (one by one or imported from a file). They belong to that group only and
-- never enter the academy's question bank.
CREATE TABLE IF NOT EXISTS group_exam_questions (
    id           text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    group_id     text NOT NULL REFERENCES group_exams (id) ON DELETE CASCADE,
    position     integer NOT NULL,
    text         text NOT NULL,
    options      text[] NOT NULL,
    answer_index integer NOT NULL,
    explanation  text NOT NULL DEFAULT '',
    UNIQUE (group_id, position)
);

ALTER TABLE group_exams ALTER COLUMN question_ids SET DEFAULT '{}';
