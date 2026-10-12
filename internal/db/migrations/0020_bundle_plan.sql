-- One online payment can buy both plans (academic + training).
ALTER TABLE chapa_payments DROP CONSTRAINT chapa_payments_module_check;
ALTER TABLE chapa_payments ADD CONSTRAINT chapa_payments_module_check CHECK (module IN ('academic', 'training', 'both'));
