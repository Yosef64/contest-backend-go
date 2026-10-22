-- Challenges: a day-by-day study plan for one grade and stream. Every day
-- has a lesson, a short note and a quiz (each optional); every seventh day is
-- the week's exam. A day opens once the day before it is finished.

-- Quizzes, weekly exams and model exams are self-paced exams like the past
-- national sets: timed from the moment a student starts, scored at once.
ALTER TABLE contests DROP CONSTRAINT IF EXISTS contests_kind_check;
ALTER TABLE contests ADD CONSTRAINT contests_kind_check
    CHECK (kind IN ('contest', 'national', 'model', 'challenge'));
ALTER TABLE contests DROP CONSTRAINT IF EXISTS contests_national_check;
ALTER TABLE contests ADD CONSTRAINT contests_national_check
    CHECK (kind = 'contest' OR (duration_minutes IS NOT NULL AND (kind <> 'national' OR exam_year IS NOT NULL)));

CREATE TABLE IF NOT EXISTS challenges (
    id          text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    title       text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    grade       text        NOT NULL CHECK (grade IN ('9', '10', '11', '12')),
    -- Empty: both streams.
    stream      text        NOT NULL DEFAULT '' CHECK (stream IN ('', 'natural', 'social')),
    weeks       integer     NOT NULL CHECK (weeks BETWEEN 1 AND 104),
    -- The planned calendar (day 1's date); students still go at their pace.
    starts_on   date,
    status      text        NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),

    -- Access for students without a paid plan (paid students open every day).
    -- Days 1..free_days are open to everyone.
    free_days        integer NOT NULL DEFAULT 0 CHECK (free_days >= 0),
    -- Inviting two new students unlocks a lesson day, or a whole week.
    referral_enabled boolean NOT NULL DEFAULT false,
    referral_unit    text    NOT NULL DEFAULT 'week' CHECK (referral_unit IN ('day', 'week')),
    -- The same for the weekly exams (one unlock per exam).
    exam_referral_enabled boolean NOT NULL DEFAULT false,
    -- From this day on only paid students get in; invites no longer unlock.
    premium_from_day integer CHECK (premium_from_day >= 1),
    -- Model and past exams at the end: 'free', 'referral' or 'premium'.
    final_access text NOT NULL DEFAULT 'premium' CHECK (final_access IN ('free', 'referral', 'premium')),

    position    integer     NOT NULL DEFAULT 0,
    created_by  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS challenges_grade_idx ON challenges (grade, position);

CREATE TABLE IF NOT EXISTS challenge_days (
    challenge_id text    NOT NULL REFERENCES challenges (id) ON DELETE CASCADE,
    day          integer NOT NULL CHECK (day >= 1),
    title        text    NOT NULL DEFAULT '',
    subject      text    NOT NULL DEFAULT '',
    minutes      integer NOT NULL DEFAULT 30 CHECK (minutes BETWEEN 1 AND 600),
    lesson_html  text    NOT NULL DEFAULT '',
    note_html    text    NOT NULL DEFAULT '',
    -- The day's quiz, or on day 7, 14, ... the week's exam.
    quiz_id      text    REFERENCES contests (id) ON DELETE SET NULL,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    updated_by   text    NOT NULL DEFAULT '',
    PRIMARY KEY (challenge_id, day)
);
CREATE UNIQUE INDEX IF NOT EXISTS challenge_days_quiz ON challenge_days (quiz_id) WHERE quiz_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS challenge_members (
    challenge_id text        NOT NULL REFERENCES challenges (id) ON DELETE CASCADE,
    student_id   text        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    joined_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (challenge_id, student_id)
);

-- Steps a student has finished; a quiz counts once it is submitted.
CREATE TABLE IF NOT EXISTS challenge_steps (
    challenge_id text        NOT NULL,
    day          integer     NOT NULL,
    student_id   text        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    step         text        NOT NULL CHECK (step IN ('lesson', 'note')),
    done_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (challenge_id, day, student_id, step),
    FOREIGN KEY (challenge_id, day) REFERENCES challenge_days (challenge_id, day) ON DELETE CASCADE
);

-- A new student who joined through a shared link. Each new student counts
-- once, for one unlock: a lesson day or week, a weekly exam, or the final
-- exams (unit 0).
CREATE TABLE IF NOT EXISTS challenge_referrals (
    challenge_id text        NOT NULL REFERENCES challenges (id) ON DELETE CASCADE,
    owner_id     text        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    visitor_id   text        NOT NULL UNIQUE REFERENCES students (id) ON DELETE CASCADE,
    kind         text        NOT NULL CHECK (kind IN ('lesson', 'exam', 'final')),
    unit         integer     NOT NULL CHECK (unit >= 0),
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS challenge_referrals_owner ON challenge_referrals (owner_id, challenge_id, kind, unit);

-- Model and past exams offered once the challenge is finished.
CREATE TABLE IF NOT EXISTS challenge_final_exams (
    challenge_id text    NOT NULL REFERENCES challenges (id) ON DELETE CASCADE,
    contest_id   text    NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    position     integer NOT NULL,
    PRIMARY KEY (challenge_id, contest_id)
);

-- Switch for every challenge's invite unlocks at once (admin Settings).
INSERT INTO site_settings (key, value) VALUES ('challenge_referrals', 'on') ON CONFLICT (key) DO NOTHING;
