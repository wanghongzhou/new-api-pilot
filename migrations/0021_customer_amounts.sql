ALTER TABLE customer
  ADD COLUMN contract_amount DECIMAL(38,10) NOT NULL DEFAULT 0 AFTER remark,
  ADD COLUMN payment_amount DECIMAL(38,10) NOT NULL DEFAULT 0 AFTER contract_amount;

ALTER TABLE customer
  ADD CONSTRAINT chk_customer_amounts_non_negative CHECK (contract_amount >= 0 AND payment_amount >= 0);
