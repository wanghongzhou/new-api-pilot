ALTER TABLE alert_rule
  MODIFY COLUMN threshold_value DECIMAL(30,2) NULL;

ALTER TABLE alert_event
  MODIFY COLUMN threshold_value DECIMAL(30,2) NULL;
