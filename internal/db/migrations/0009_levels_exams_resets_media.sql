-- Levels: grades 9-11 are high school with no academy department. Grade 12
-- is the entrance department; remedial and freshman are university levels
-- (stored with grade '12' so they keep the grade 12 content).
ALTER TABLE academic_profiles DROP CONSTRAINT academic_profiles_department_check;
ALTER TABLE academic_profiles ADD CONSTRAINT academic_profiles_department_check
    CHECK (department IN ('', 'entrance', 'remedial', 'freshman'));
ALTER TABLE academic_profiles ALTER COLUMN department SET DEFAULT '';
ALTER TABLE academic_profiles DROP CONSTRAINT academic_profiles_stream_check;
ALTER TABLE academic_profiles ADD CONSTRAINT academic_profiles_stream_check
    CHECK (stream IN ('', 'natural', 'social'));
ALTER TABLE academic_profiles ALTER COLUMN stream SET DEFAULT '';

UPDATE academic_profiles p SET department = '', updated_at = now()
    FROM students s WHERE s.id = p.student_id AND s.grade IN ('9', '10', '11') AND p.department = 'entrance';
UPDATE academic_profiles p SET stream = '', updated_at = now()
    FROM students s WHERE s.id = p.student_id AND s.grade IN ('9', '10') AND p.department = '';
UPDATE students s SET grade = '12', updated_at = now()
    FROM academic_profiles p WHERE p.student_id = s.id AND p.department IN ('entrance', 'remedial', 'freshman') AND s.grade <> '12';

-- Exams: per-exam rules and scoring shown before starting, and past national
-- entrance exam sets taken as timed practice.
ALTER TABLE contests ADD COLUMN kind text NOT NULL DEFAULT 'contest' CHECK (kind IN ('contest', 'national'));
ALTER TABLE contests ADD COLUMN exam_year integer CHECK (exam_year BETWEEN 1990 AND 2100);
ALTER TABLE contests ADD COLUMN stream text NOT NULL DEFAULT '' CHECK (stream IN ('', 'natural', 'social'));
ALTER TABLE contests ADD COLUMN duration_minutes integer CHECK (duration_minutes BETWEEN 1 AND 600);
ALTER TABLE contests ADD COLUMN rules text NOT NULL DEFAULT '';
ALTER TABLE contests ADD COLUMN scoring text NOT NULL DEFAULT '';
ALTER TABLE contests ADD CONSTRAINT contests_national_check
    CHECK (kind = 'contest' OR (exam_year IS NOT NULL AND duration_minutes IS NOT NULL));
CREATE INDEX contests_kind_idx ON contests (kind, exam_year DESC);

-- One timed attempt per student; resuming keeps the original deadline.
CREATE TABLE practice_attempts (
    contest_id text        NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    student_id text        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    started_at timestamptz NOT NULL DEFAULT now(),
    ends_at    timestamptz NOT NULL,
    PRIMARY KEY (contest_id, student_id)
);

-- Forgot password: one active code per login, stored hashed.
CREATE TABLE password_resets (
    login      text PRIMARY KEY REFERENCES student_credentials (login) ON DELETE CASCADE,
    code_hash  text        NOT NULL,
    expires_at timestamptz NOT NULL,
    attempts   integer     NOT NULL DEFAULT 0,
    issued_by  text        REFERENCES admins (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Metadata for every uploaded file and question.
CREATE TABLE media_uploads (
    id            text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    url           text        NOT NULL,
    kind          text        NOT NULL CHECK (kind IN ('image', 'video')),
    content_type  text        NOT NULL,
    file_name     text        NOT NULL DEFAULT '',
    size_bytes    bigint      NOT NULL CHECK (size_bytes >= 0),
    uploaded_by   text        REFERENCES admins (id) ON DELETE SET NULL,
    uploader_name text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX media_uploads_created_idx ON media_uploads (created_at DESC);

ALTER TABLE questions ADD COLUMN created_by text NOT NULL DEFAULT '';
ALTER TABLE questions ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE questions ADD COLUMN updated_by text NOT NULL DEFAULT '';
UPDATE questions SET updated_at = created_at;
ALTER TABLE contests ADD COLUMN created_by text NOT NULL DEFAULT '';
ALTER TABLE contests ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
UPDATE contests SET updated_at = created_at;
