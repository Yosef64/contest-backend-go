-- Victory v1 schema. Grades are stored as '9'..'12' (the old mini app used
-- '1'..'4'); question answers are stored 0-based.

CREATE TABLE admins (
    id            text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name          text        NOT NULL,
    email         text        NOT NULL,
    password_hash text        NOT NULL,
    is_approved   boolean     NOT NULL DEFAULT false,
    is_owner      boolean     NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX admins_email_key ON admins (lower(email));

CREATE TABLE students (
    id            text PRIMARY KEY,
    auth_provider text        NOT NULL CHECK (auth_provider IN ('google', 'telegram', 'dev')),
    auth_subject  text        NOT NULL,
    name          text        NOT NULL,
    phone_number  text        NOT NULL DEFAULT '',
    gender        text        NOT NULL DEFAULT '',
    age           integer     NOT NULL DEFAULT 0 CHECK (age >= 0),
    school        text        NOT NULL DEFAULT '',
    grade         text        NOT NULL CHECK (grade IN ('9', '10', '11', '12')),
    city          text        NOT NULL DEFAULT '',
    region        text        NOT NULL DEFAULT '',
    photo_url     text        NOT NULL DEFAULT '',
    is_suspended  boolean     NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (auth_provider, auth_subject)
);
CREATE INDEX students_grade_idx ON students (grade);
CREATE INDEX students_created_idx ON students (created_at DESC);

CREATE TABLE courses (
    id          text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    grade       text        NOT NULL CHECK (grade IN ('9', '10', '11', '12')),
    subject     text        NOT NULL,
    title       text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    position    integer     NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX courses_grade_idx ON courses (grade, position);

CREATE TABLE chapters (
    id              text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    course_id       text        NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    position        integer     NOT NULL,
    title           text        NOT NULL,
    summary         text        NOT NULL DEFAULT '',
    minutes         integer     NOT NULL DEFAULT 15 CHECK (minutes BETWEEN 1 AND 240),
    html            text        NOT NULL DEFAULT '',
    status          text        NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    version         integer     NOT NULL DEFAULT 1,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    updated_by      text        NOT NULL DEFAULT '',
    lock_admin_id   text,
    lock_admin_name text,
    lock_expires_at timestamptz
);
CREATE INDEX chapters_course_idx ON chapters (course_id, position);

CREATE TABLE chapter_versions (
    chapter_id text        NOT NULL REFERENCES chapters (id) ON DELETE CASCADE,
    version    integer     NOT NULL,
    title      text        NOT NULL,
    html       text        NOT NULL,
    author     text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (chapter_id, version)
);

CREATE TABLE chapter_comments (
    id          text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    chapter_id  text        NOT NULL REFERENCES chapters (id) ON DELETE CASCADE,
    author_id   text        NOT NULL,
    author_name text        NOT NULL,
    body        text        NOT NULL,
    quote       text        NOT NULL DEFAULT '',
    resolved    boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX chapter_comments_chapter_idx ON chapter_comments (chapter_id, created_at);

CREATE TABLE chapter_progress (
    student_id   text        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    chapter_id   text        NOT NULL REFERENCES chapters (id) ON DELETE CASCADE,
    completed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (student_id, chapter_id)
);

CREATE TABLE questions (
    id           text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    text         text        NOT NULL,
    options      text[]      NOT NULL CHECK (cardinality(options) BETWEEN 2 AND 6),
    answer_index integer     NOT NULL CHECK (answer_index >= 0),
    explanation  text        NOT NULL DEFAULT '',
    subject      text        NOT NULL,
    chapter      text        NOT NULL,
    grade        text        NOT NULL CHECK (grade IN ('9', '10', '11', '12')),
    image_url    text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    CHECK (answer_index < cardinality(options))
);
CREATE INDEX questions_subject_grade_idx ON questions (subject, grade);

CREATE TABLE contests (
    id          text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    title       text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    subject     text        NOT NULL,
    grade       text        NOT NULL CHECK (grade IN ('9', '10', '11', '12')),
    start_time  timestamptz NOT NULL,
    end_time    timestamptz NOT NULL,
    prize       text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (end_time > start_time)
);
CREATE INDEX contests_start_idx ON contests (start_time);

CREATE TABLE contest_questions (
    contest_id  text    NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    question_id text    NOT NULL REFERENCES questions (id) ON DELETE RESTRICT,
    position    integer NOT NULL,
    PRIMARY KEY (contest_id, question_id)
);

CREATE TABLE registrations (
    contest_id    text        NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    student_id    text        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    subjects      text[]      NOT NULL DEFAULT '{}',
    experience    text        NOT NULL DEFAULT '',
    registered_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (contest_id, student_id)
);
CREATE INDEX registrations_registered_idx ON registrations (registered_at);

CREATE TABLE submissions (
    id           text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    contest_id   text        NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    student_id   text        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    answers      jsonb       NOT NULL DEFAULT '{}',
    correct      integer     NOT NULL,
    total        integer     NOT NULL,
    time_seconds integer     NOT NULL DEFAULT 0,
    submitted_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (contest_id, student_id)
);
CREATE INDEX submissions_student_idx ON submissions (student_id);
