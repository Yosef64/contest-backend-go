-- Field agents bring in students with their own code and link (/a/CODE)
-- and earn a commission on every plan their students take.
ALTER TABLE field_agents
    ADD COLUMN IF NOT EXISTS code text UNIQUE,
    -- Percent of each plan's price.
    ADD COLUMN IF NOT EXISTS commission_rate numeric(5, 2) NOT NULL DEFAULT 10
        CHECK (commission_rate >= 0 AND commission_rate <= 100);

-- Which agent brought a student in (once, at most).
CREATE TABLE IF NOT EXISTS agent_referrals (
    student_id text PRIMARY KEY REFERENCES students(id) ON DELETE CASCADE,
    agent_id   text NOT NULL REFERENCES field_agents(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_referrals_agent ON agent_referrals (agent_id, created_at DESC);

-- One commission per approved plan of a referred student.
CREATE TABLE IF NOT EXISTS agent_commissions (
    id            bigserial PRIMARY KEY,
    agent_id      text NOT NULL REFERENCES field_agents(id) ON DELETE CASCADE,
    enrollment_id text NOT NULL UNIQUE REFERENCES enrollment_requests(id) ON DELETE CASCADE,
    student_id    text NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    module        text NOT NULL,
    amount        numeric(12, 2) NOT NULL,
    rate          numeric(5, 2) NOT NULL,
    status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    paid_at       timestamptz
);
CREATE INDEX IF NOT EXISTS agent_commissions_agent ON agent_commissions (agent_id, created_at DESC);

-- Every way a plan gets approved (online payment, an admin approving a
-- payment proof) earns the commission, so it lives in the database.
CREATE OR REPLACE FUNCTION agent_commission() RETURNS trigger AS $$
DECLARE
    agent record;
    price numeric;
BEGIN
    IF NEW.status <> 'approved' OR (TG_OP = 'UPDATE' AND OLD.status = 'approved') THEN
        RETURN NEW;
    END IF;
    SELECT f.id, f.commission_rate INTO agent
        FROM agent_referrals r JOIN field_agents f ON f.id = r.agent_id AND f.status = 'approved'
        WHERE r.student_id = NEW.student_id;
    IF NOT FOUND THEN
        RETURN NEW;
    END IF;
    SELECT CASE WHEN value ~ '^[0-9]+(\.[0-9]+)?$' THEN value::numeric ELSE 0 END INTO price
        FROM site_settings WHERE key = 'price_' || NEW.module;
    INSERT INTO agent_commissions (agent_id, enrollment_id, student_id, module, amount, rate)
        VALUES (agent.id, NEW.id, NEW.student_id, NEW.module,
                round(coalesce(price, 0) * agent.commission_rate / 100, 2), agent.commission_rate)
        ON CONFLICT (enrollment_id) DO NOTHING;
    RETURN NEW;
END
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS enrollment_agent_commission ON enrollment_requests;
CREATE TRIGGER enrollment_agent_commission
    AFTER INSERT OR UPDATE OF status ON enrollment_requests
    FOR EACH ROW EXECUTE FUNCTION agent_commission();
