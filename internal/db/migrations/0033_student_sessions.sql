-- Signed-in devices. An account is signed in on at most two devices; a
-- third sign-in signs out the one used least recently.
CREATE TABLE IF NOT EXISTS student_sessions (
    id            text PRIMARY KEY,
    -- No foreign key: a first sign-in comes before the profile exists.
    student_id    text NOT NULL,
    -- The app's install ID: signing in again on a device replaces its session.
    device_id     text NOT NULL DEFAULT '',
    device        text NOT NULL DEFAULT '',
    platform      text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now(),
    signed_out_at timestamptz,
    -- device_limit, signed_out, signed_in_again, password_changed
    reason        text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS student_sessions_active ON student_sessions (student_id, last_seen_at DESC)
    WHERE signed_out_at IS NULL;
