ALTER TABLE export_job
  ADD COLUMN claim_token VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER heartbeat_at;

ALTER TABLE export_job
  ADD COLUMN lease_expires_at BIGINT NULL AFTER claim_token;

ALTER TABLE export_job
  ADD KEY idx_export_job_claim (status, lease_expires_at, next_attempt_at, id);
