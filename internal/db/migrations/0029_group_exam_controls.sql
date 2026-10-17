-- Group exam creators control their exam: exam mode (phones stay pinned to
-- Victory during the exam, no codes), ending it early, deleting it and
-- running it again. Answers stay on the phones until each student submits;
-- with exam mode on, a submission says how often the student left the app.
ALTER TABLE group_exams ADD COLUMN IF NOT EXISTS exam_mode boolean NOT NULL DEFAULT false;
ALTER TABLE group_exam_members ADD COLUMN IF NOT EXISTS breaks integer NOT NULL DEFAULT 0;
