-- AI features: a per-student daily question limit, and cached results
-- (one study analysis per student per day) so repeat visits cost nothing.
CREATE TABLE IF NOT EXISTS ai_usage (
    student_id text NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    day        date NOT NULL,
    requests   integer NOT NULL DEFAULT 0,
    PRIMARY KEY (student_id, day)
);

CREATE TABLE IF NOT EXISTS ai_cache (
    owner_id   text NOT NULL,
    kind       text NOT NULL,
    day        date NOT NULL,
    content    jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (owner_id, kind, day)
);
