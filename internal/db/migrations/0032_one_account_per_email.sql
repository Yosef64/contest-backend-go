-- One email, one Victory account, whichever way the student signs in.

-- The verified email of each Google sign-in and the account it opens. A
-- Google sign-in may come before the student row exists (profile setup), so
-- there is no foreign key; account deletion removes the row.
CREATE TABLE IF NOT EXISTS google_accounts (
    email      text PRIMARY KEY,
    student_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS google_accounts_student ON google_accounts (student_id);

-- When a password account proved it owns its email (a reset by emailed
-- code). A Google sign-in for the same email keeps only proven passwords.
ALTER TABLE student_credentials ADD COLUMN IF NOT EXISTS email_verified_at timestamptz;

-- A Google account can add a password with an emailed code, before it has
-- a password login.
ALTER TABLE password_resets DROP CONSTRAINT IF EXISTS password_resets_login_fkey;
