-- Courses for university levels too: remedial and freshman, like exams and
-- questions.
ALTER TABLE courses DROP CONSTRAINT courses_grade_check;
ALTER TABLE courses ADD CONSTRAINT courses_grade_check CHECK (grade IN ('9', '10', '11', '12', 'remedial', 'freshman'));
