CREATE TABLE exam_mode_settings (
 contest_id text PRIMARY KEY REFERENCES contests(id) ON DELETE CASCADE,
 entry_hash text NOT NULL, exit_hash text NOT NULL, enabled boolean NOT NULL DEFAULT true);
CREATE TABLE exam_mode_sessions (
 contest_id text REFERENCES contests(id) ON DELETE CASCADE,student_id text REFERENCES students(id) ON DELETE CASCADE,
 entered_at timestamptz NOT NULL DEFAULT now(),exited_at timestamptz,
 PRIMARY KEY(contest_id,student_id));
