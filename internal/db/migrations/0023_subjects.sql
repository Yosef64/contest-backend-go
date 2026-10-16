-- Subjects are managed by admins (e.g. Logic, Emerging Technologies,
-- Aptitude for freshman and remedial students). Courses, questions, exams
-- and plans refer to a subject by its name.
CREATE TABLE IF NOT EXISTS subjects (
    name       text PRIMARY KEY CHECK (length(name) BETWEEN 2 AND 60),
    -- An icon key the apps understand (sigma, atom, flask, leaf, ...).
    icon       text NOT NULL DEFAULT 'book',
    color      text NOT NULL DEFAULT '',
    -- Levels it is taught at ("9".."12", "remedial", "freshman"); empty: all.
    levels     text[] NOT NULL DEFAULT '{}',
    position   integer NOT NULL DEFAULT 0,
    active     boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO subjects (name, icon, color, position) VALUES
    ('Mathematics', 'sigma',     '#6D28D9', 1),
    ('Physics',     'atom',      '#0369A1', 2),
    ('Chemistry',   'flask',     '#0F766E', 3),
    ('Biology',     'leaf',      '#047857', 4),
    ('English',     'languages', '#C2410C', 5),
    ('History',     'landmark',  '#92400E', 6),
    ('Geography',   'globe',     '#1D4ED8', 7),
    ('Economics',   'trending',  '#B45309', 8),
    ('Civics',      'scale',     '#4338CA', 9)
ON CONFLICT (name) DO NOTHING;
