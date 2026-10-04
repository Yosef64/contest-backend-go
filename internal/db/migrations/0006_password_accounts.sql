-- Email / phone + password accounts for students, alongside Google and
-- Telegram. The student row itself is still created when the profile is
-- completed (PUT /me); credentials only hold the login and password.
ALTER TABLE students DROP CONSTRAINT students_auth_provider_check;
ALTER TABLE students ADD CONSTRAINT students_auth_provider_check
    CHECK (auth_provider IN ('google', 'telegram', 'dev', 'password'));

CREATE TABLE student_credentials (
    login               text PRIMARY KEY, -- lower-case email or +251 phone
    student_id          text        NOT NULL UNIQUE,
    display_name        text        NOT NULL,
    password_hash       text        NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    password_changed_at timestamptz NOT NULL DEFAULT now()
);
