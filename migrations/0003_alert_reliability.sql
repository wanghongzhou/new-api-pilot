CREATE TABLE IF NOT EXISTS alert_evaluation_cursor (
  active_key VARCHAR(384) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  last_sample_at BIGINT NOT NULL,
  last_sample_key VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (active_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE alert_delivery
  ADD COLUMN claim_token VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER attempt_count,
  ADD COLUMN lease_expires_at BIGINT NULL AFTER claim_token,
  ADD COLUMN payload_snapshot JSON NULL AFTER lease_expires_at,
  ADD KEY idx_alert_delivery_claim (status, lease_expires_at, next_retry_at, id);

UPDATE alert_delivery
SET payload_snapshot = CASE
  WHEN event_type = 'test' THEN JSON_OBJECT('version', 1, 'kind', 'test')
  ELSE JSON_OBJECT(
    'version', 1,
    'kind', 'legacy',
    'alert_event_id', CAST(alert_event_id AS CHAR),
    'event_type', event_type
  )
END
WHERE payload_snapshot IS NULL;

ALTER TABLE alert_delivery
  MODIFY COLUMN payload_snapshot JSON NOT NULL AFTER lease_expires_at;
