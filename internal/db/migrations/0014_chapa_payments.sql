-- Online payments through Chapa. A paid transaction approves the student's
-- enrollment straight away (no manual proof check needed).
CREATE TABLE IF NOT EXISTS chapa_payments (
    tx_ref        text PRIMARY KEY,
    student_id    text NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    module        text NOT NULL CHECK (module IN ('academic', 'training')),
    amount        numeric(12, 2) NOT NULL,
    currency      text NOT NULL DEFAULT 'ETB',
    status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'success', 'failed')),
    checkout_url  text NOT NULL DEFAULT '',
    chapa_ref     text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    verified_at   timestamptz
);
CREATE INDEX IF NOT EXISTS chapa_payments_student_idx ON chapa_payments (student_id, created_at DESC);
