-- Topics: a finer level under a question's chapter, so school reports can
-- break results down by subject, chapter and topic.
ALTER TABLE questions ADD COLUMN IF NOT EXISTS topic text NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS questions_subject_chapter_topic_idx ON questions (subject, chapter, topic);
