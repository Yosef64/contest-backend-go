-- Closes ways an agent's commission could be earned without bringing in a
-- new paying student:
-- - only plans requested after the student joined through the agent count
--   (not one that was already paid or waiting for review);
-- - never on a student account with the agent's own phone number;
-- - the amount is what the student paid online (a "both" plan bought with
--   Chapa is shared between its two plans), or the plan's price for a
--   reviewed payment proof;
-- - a plan that is later rejected or deleted cancels its unpaid commission,
--   and a plan whose program is changed is recalculated;
-- - commission records are kept when a plan record is deleted.
ALTER TABLE agent_commissions DROP CONSTRAINT IF EXISTS agent_commissions_status_check;
ALTER TABLE agent_commissions ADD CONSTRAINT agent_commissions_status_check
    CHECK (status IN ('pending', 'paid', 'cancelled'));
ALTER TABLE agent_commissions ALTER COLUMN enrollment_id DROP NOT NULL;
ALTER TABLE agent_commissions DROP CONSTRAINT IF EXISTS agent_commissions_enrollment_id_fkey;
ALTER TABLE agent_commissions ADD CONSTRAINT agent_commissions_enrollment_id_fkey
    FOREIGN KEY (enrollment_id) REFERENCES enrollment_requests(id) ON DELETE SET NULL;

-- The last 9 digits of a phone number (0911…, +251911… and 251911… match).
CREATE OR REPLACE FUNCTION phone_key(p text) RETURNS text AS $$
    SELECT nullif(right(regexp_replace(coalesce(p, ''), '\D', '', 'g'), 9), '')
$$ LANGUAGE sql IMMUTABLE;

-- What the agent earns for one approved plan.
CREATE OR REPLACE FUNCTION agent_commission_amount(e enrollment_requests, rate numeric) RETURNS numeric AS $$
DECLARE
    price numeric;
    paid record;
BEGIN
    IF e.reference LIKE 'Chapa %' THEN
        SELECT amount, module INTO paid FROM chapa_payments
            WHERE tx_ref = substr(e.reference, 7) AND status = 'success';
        IF FOUND THEN
            price := paid.amount / CASE WHEN paid.module = 'both' THEN 2 ELSE 1 END;
        END IF;
    END IF;
    IF price IS NULL THEN
        SELECT CASE WHEN value ~ '^[0-9]+(\.[0-9]+)?$' THEN value::numeric ELSE 0 END INTO price
            FROM site_settings WHERE key = 'price_' || e.module;
    END IF;
    RETURN round(coalesce(price, 0) * rate / 100, 2);
END
$$ LANGUAGE plpgsql STABLE;

CREATE OR REPLACE FUNCTION agent_commission() RETURNS trigger AS $$
DECLARE
    agent record;
BEGIN
    IF TG_OP = 'DELETE' THEN
        UPDATE agent_commissions SET status = 'cancelled'
            WHERE enrollment_id = OLD.id AND status = 'pending';
        RETURN OLD;
    END IF;
    IF NEW.status <> 'approved' THEN
        -- Rejected or back to review: what isn't paid out yet is cancelled.
        IF TG_OP = 'UPDATE' AND OLD.status = 'approved' THEN
            UPDATE agent_commissions SET status = 'cancelled'
                WHERE enrollment_id = NEW.id AND status = 'pending';
        END IF;
        RETURN NEW;
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.status = 'approved' THEN
        -- Still approved: an unpaid commission follows a program change.
        IF NEW.module IS DISTINCT FROM OLD.module THEN
            UPDATE agent_commissions m SET module = NEW.module,
                    amount = agent_commission_amount(NEW, m.rate)
                WHERE m.enrollment_id = NEW.id AND m.status = 'pending';
        END IF;
        RETURN NEW;
    END IF;
    SELECT f.id, f.commission_rate INTO agent
        FROM agent_referrals r
        JOIN field_agents f ON f.id = r.agent_id AND f.status = 'approved'
        JOIN students s ON s.id = r.student_id
        WHERE r.student_id = NEW.student_id
            -- Requested after the student joined through the agent.
            AND NEW.created_at >= r.created_at
            -- Not the agent's own second account.
            AND (f.student_id IS NULL OR f.student_id <> s.id)
            AND phone_key(s.phone_number) IS DISTINCT FROM phone_key(f.phone)
            AND (f.student_id IS NULL OR phone_key(s.phone_number) IS DISTINCT FROM
                 (SELECT phone_key(phone_number) FROM students WHERE id = f.student_id));
    IF NOT FOUND THEN
        RETURN NEW;
    END IF;
    INSERT INTO agent_commissions (agent_id, enrollment_id, student_id, module, amount, rate)
        VALUES (agent.id, NEW.id, NEW.student_id, NEW.module,
                agent_commission_amount(NEW, agent.commission_rate), agent.commission_rate)
        -- Approved again after being rejected: earned again.
        ON CONFLICT (enrollment_id) DO UPDATE SET status = 'pending', module = EXCLUDED.module,
                amount = EXCLUDED.amount, created_at = now()
            WHERE agent_commissions.status = 'cancelled';
    RETURN NEW;
END
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS enrollment_agent_commission ON enrollment_requests;
CREATE TRIGGER enrollment_agent_commission
    AFTER INSERT OR UPDATE OF status, module ON enrollment_requests
    FOR EACH ROW EXECUTE FUNCTION agent_commission();
-- Before the row goes, while its commission can still be found.
DROP TRIGGER IF EXISTS enrollment_agent_commission_delete ON enrollment_requests;
CREATE TRIGGER enrollment_agent_commission_delete
    BEFORE DELETE ON enrollment_requests
    FOR EACH ROW EXECUTE FUNCTION agent_commission();
