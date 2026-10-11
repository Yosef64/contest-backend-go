-- Questions use the same levels as exams (grade 9-12, remedial, freshman)
-- and, from grade 11 up, a stream. An empty stream suits both streams.
ALTER TABLE questions DROP CONSTRAINT questions_grade_check;
ALTER TABLE questions ADD CONSTRAINT questions_grade_check CHECK (grade IN ('9', '10', '11', '12', 'remedial', 'freshman'));
ALTER TABLE questions ADD COLUMN stream text NOT NULL DEFAULT '' CHECK (stream IN ('', 'natural', 'social'));
