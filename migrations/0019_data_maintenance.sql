CREATE TABLE IF NOT EXISTS data_maintenance_state (
  id BIGINT NOT NULL AUTO_INCREMENT,
  operation_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  scope_key VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  scope_revision CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  date_key INT NOT NULL DEFAULT 0,
  status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  cursor_kind VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  cursor_site_id BIGINT NOT NULL DEFAULT 0,
  cursor_node_name VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  cursor_bucket_start BIGINT NOT NULL DEFAULT 0,
  cursor_id BIGINT NOT NULL DEFAULT 0,
  site_id BIGINT NULL,
  site_config_version BIGINT NULL,
  request_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  run_id BIGINT NULL,
  error_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  attempt_count INT NOT NULL DEFAULT 0,
  next_attempt_at BIGINT NOT NULL DEFAULT 0,
  last_attempt_at BIGINT NULL,
  last_success_at BIGINT NULL,
  last_failure_at BIGINT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_data_maintenance_scope (operation_id, scope_key),
  KEY idx_data_maintenance_due (status, next_attempt_at, id),
  KEY idx_data_maintenance_site (site_id, site_config_version, operation_id),
  CONSTRAINT fk_data_maintenance_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT fk_data_maintenance_run FOREIGN KEY (run_id) REFERENCES collection_run(id)
    ON UPDATE RESTRICT ON DELETE SET NULL,
  CONSTRAINT chk_data_maintenance_operation CHECK (operation_id IN (
    'authorize_pricing_group_sync', 'resource_daily_finalize',
    'resource_rollup_gap_repair', 'collection_run_error_redaction',
    'metadata_diagnostic_run_cleanup'
  )),
  CONSTRAINT chk_data_maintenance_status CHECK (status IN ('pending', 'running', 'complete', 'failed')),
  CONSTRAINT chk_data_maintenance_cursor CHECK (
    date_key >= 0 AND cursor_site_id >= 0 AND cursor_bucket_start >= 0 AND cursor_id >= 0
  ),
  CONSTRAINT chk_data_maintenance_attempt CHECK (
    attempt_count >= 0 AND next_attempt_at >= 0 AND created_at > 0 AND updated_at >= created_at AND
    (last_attempt_at IS NULL OR last_attempt_at >= 0) AND
    (last_success_at IS NULL OR last_success_at >= 0) AND
    (last_failure_at IS NULL OR last_failure_at >= 0)
  ),
  CONSTRAINT chk_data_maintenance_site_pair CHECK (
    (site_id IS NULL AND site_config_version IS NULL) OR
    (site_id > 0 AND site_config_version > 0)
  ),
  CONSTRAINT chk_data_maintenance_shape CHECK (
    (
      operation_id = 'authorize_pricing_group_sync' AND date_key = 0 AND
      site_id IS NOT NULL AND site_config_version IS NOT NULL AND
      scope_key = CONCAT('site:', site_id, ':config:', site_config_version) AND
      scope_revision = '' AND request_id <> ''
    ) OR (
      operation_id IN ('resource_daily_finalize','resource_rollup_gap_repair') AND
      scope_key = 'global' AND site_id IS NULL AND site_config_version IS NULL AND
      request_id = '' AND
      ((date_key = 0 AND status = 'pending' AND scope_revision = '') OR
       (date_key > 0 AND scope_revision REGEXP '^[0-9a-f]{64}$'))
    ) OR (
      operation_id IN ('collection_run_error_redaction','metadata_diagnostic_run_cleanup') AND
      scope_key = 'global' AND site_id IS NULL AND site_config_version IS NULL AND
      request_id = '' AND scope_revision = ''
    )
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
