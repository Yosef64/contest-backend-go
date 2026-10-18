-- Students can delete their account (Google Play requires it). Their
-- learning data goes with them (ON DELETE CASCADE); payments, plans and
-- agents' commissions stay for the academy's accounts, with the student
-- removed (ON DELETE SET NULL).
ALTER TABLE enrollment_requests ALTER COLUMN student_id DROP NOT NULL;
ALTER TABLE enrollment_requests DROP CONSTRAINT IF EXISTS enrollment_requests_student_id_fkey;
ALTER TABLE enrollment_requests ADD CONSTRAINT enrollment_requests_student_id_fkey
    FOREIGN KEY (student_id) REFERENCES students (id) ON DELETE SET NULL;

ALTER TABLE chapa_payments ALTER COLUMN student_id DROP NOT NULL;
ALTER TABLE chapa_payments DROP CONSTRAINT IF EXISTS chapa_payments_student_id_fkey;
ALTER TABLE chapa_payments ADD CONSTRAINT chapa_payments_student_id_fkey
    FOREIGN KEY (student_id) REFERENCES students (id) ON DELETE SET NULL;

ALTER TABLE agent_commissions ALTER COLUMN student_id DROP NOT NULL;
ALTER TABLE agent_commissions DROP CONSTRAINT IF EXISTS agent_commissions_student_id_fkey;
ALTER TABLE agent_commissions ADD CONSTRAINT agent_commissions_student_id_fkey
    FOREIGN KEY (student_id) REFERENCES students (id) ON DELETE SET NULL;
