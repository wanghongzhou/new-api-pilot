UPDATE alert_rule
SET threshold_value = 0.90, updated_at = UNIX_TIMESTAMP()
WHERE rule_key = 'channel_availability_low'
  AND level = 'warning'
  AND scope_type = 'global'
  AND scope_id = 0;

UPDATE alert_rule
SET threshold_value = 0.80, updated_at = UNIX_TIMESTAMP()
WHERE rule_key = 'channel_availability_low'
  AND level = 'critical'
  AND scope_type = 'global'
  AND scope_id = 0;
