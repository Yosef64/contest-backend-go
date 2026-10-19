-- Each payout to an agent: how much, where it was sent (the details as they
-- were that day), the transfer reference, and who paid. Commissions point to
-- the payout that settled them.
CREATE TABLE IF NOT EXISTS agent_payouts (
    id           bigserial PRIMARY KEY,
    agent_id     text NOT NULL REFERENCES field_agents(id) ON DELETE CASCADE,
    amount       numeric(12, 2) NOT NULL,
    commissions  integer NOT NULL,
    method       text NOT NULL DEFAULT '',
    bank         text NOT NULL DEFAULT '',
    account      text NOT NULL DEFAULT '',
    account_name text NOT NULL DEFAULT '',
    reference    text NOT NULL DEFAULT '',
    paid_by      text REFERENCES admins(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_payouts_agent ON agent_payouts (agent_id, created_at DESC);

ALTER TABLE agent_commissions
    ADD COLUMN IF NOT EXISTS payout_id bigint REFERENCES agent_payouts(id) ON DELETE SET NULL;

-- Commissions paid before payouts were recorded: one record per agent.
DO $$
DECLARE
    a record;
    pid bigint;
BEGIN
    FOR a IN SELECT agent_id, sum(amount) AS amount, count(*)::int AS n, max(paid_at) AS at
             FROM agent_commissions WHERE status = 'paid' AND payout_id IS NULL GROUP BY agent_id
    LOOP
        INSERT INTO agent_payouts (agent_id, amount, commissions, reference, created_at)
            VALUES (a.agent_id, a.amount, a.n, 'Paid before payout records', coalesce(a.at, now()))
            RETURNING id INTO pid;
        UPDATE agent_commissions SET payout_id = pid
            WHERE agent_id = a.agent_id AND status = 'paid' AND payout_id IS NULL;
    END LOOP;
END
$$;
