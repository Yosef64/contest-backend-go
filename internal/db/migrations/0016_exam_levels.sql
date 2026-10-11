-- Existing grade values and exam data stay unchanged.
ALTER TABLE contests DROP CONSTRAINT contests_grade_check;
ALTER TABLE contests ADD CONSTRAINT contests_grade_check CHECK (grade IN ('9', '10', '11', '12', 'remedial', 'freshman'));
