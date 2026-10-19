-- Where an agent's commission is sent: a bank account or a mobile wallet.
-- payout_updated_at lets the admin notice details changed just before a
-- payout (someone else may have changed them).
ALTER TABLE field_agents
    ADD COLUMN IF NOT EXISTS payout_method text NOT NULL DEFAULT ''
        CHECK (payout_method IN ('', 'bank', 'telebirr', 'cbe_birr')),
    ADD COLUMN IF NOT EXISTS payout_bank text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payout_account text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payout_name text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payout_updated_at timestamptz;
