-- Partner schools' own content: a school's admins and teachers write
-- questions and exams that only that school sees (and that its students
-- take free of charge). Main-office content has no school.
ALTER TABLE questions ADD COLUMN IF NOT EXISTS school_id text REFERENCES schools(id) ON DELETE CASCADE;
ALTER TABLE contests ADD COLUMN IF NOT EXISTS school_id text REFERENCES schools(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS questions_school_idx ON questions (school_id) WHERE school_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS contests_school_idx ON contests (school_id) WHERE school_id IS NOT NULL;

-- Teachers: like a school admin (manager), for one school.
ALTER TABLE admins DROP CONSTRAINT IF EXISTS admins_permission_role_check;
ALTER TABLE admins ADD CONSTRAINT admins_permission_role_check
    CHECK (permission_role IN ('editor', 'analyst', 'manager', 'teacher', 'uploader'));
