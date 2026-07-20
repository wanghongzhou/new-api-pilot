CREATE TABLE IF NOT EXISTS schema_migration (
  version VARCHAR(64) NOT NULL,
  checksum CHAR(64) NOT NULL,
  applied_at BIGINT NOT NULL,
  PRIMARY KEY (version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS platform_user (
  id BIGINT NOT NULL AUTO_INCREMENT,
  username VARCHAR(64) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  display_name VARCHAR(128) NOT NULL DEFAULT '',
  role VARCHAR(16) NOT NULL,
  status TINYINT NOT NULL DEFAULT 1,
  must_change_password TINYINT NOT NULL DEFAULT 0,
  session_version INT NOT NULL DEFAULT 1,
  last_login_at BIGINT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_platform_user_username (username),
  KEY idx_platform_user_role_status (role, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site (
  id BIGINT NOT NULL AUTO_INCREMENT,
  name VARCHAR(128) NOT NULL,
  base_url VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  config_version INT NOT NULL DEFAULT 1,
  remark VARCHAR(500) NOT NULL DEFAULT '',
  management_status VARCHAR(16) NOT NULL DEFAULT 'active',
  online_status VARCHAR(16) NOT NULL DEFAULT 'unknown',
  auth_status VARCHAR(16) NOT NULL DEFAULT 'unauthorized',
  statistics_status VARCHAR(32) NOT NULL DEFAULT 'pending_config',
  health_status VARCHAR(16) NOT NULL DEFAULT 'unavailable',
  root_user_id BIGINT NULL,
  root_created_at BIGINT NULL,
  access_token_encrypted MEDIUMTEXT NULL,
  version VARCHAR(64) NOT NULL DEFAULT '',
  system_name VARCHAR(128) NOT NULL DEFAULT '',
  quota_per_unit DECIMAL(30,10) NULL,
  usd_exchange_rate DECIMAL(30,10) NULL,
  last_rate_at BIGINT NULL,
  data_export_enabled TINYINT NOT NULL DEFAULT 0,
  current_rpm BIGINT NOT NULL DEFAULT 0,
  current_tpm BIGINT NOT NULL DEFAULT 0,
  last_realtime_stat_at BIGINT NULL,
  probe_fail_count INT NOT NULL DEFAULT 0,
  last_probe_at BIGINT NULL,
  last_probe_success_at BIGINT NULL,
  statistics_start_at BIGINT NULL,
  statistics_start_source VARCHAR(16) NULL,
  statistics_end_at BIGINT NULL,
  disabled_at BIGINT NULL,
  monitoring_start_at BIGINT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_site_base_url (base_url),
  KEY idx_site_status (management_status, online_status, auth_status),
  KEY idx_site_statistics_status (statistics_status, health_status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_channel (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  remote_channel_id BIGINT NOT NULL,
  name VARCHAR(255) NOT NULL,
  last_synced_at BIGINT NOT NULL,
  remote_missing TINYINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_site_channel_remote (site_id, remote_channel_id),
  KEY idx_site_channel_name (site_id, name),
  CONSTRAINT fk_site_channel_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_monitoring_pause (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  start_minute_ts BIGINT NOT NULL,
  end_minute_ts BIGINT NULL,
  reason VARCHAR(32) NOT NULL DEFAULT 'management_disabled',
  created_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_site_monitoring_pause (site_id, start_minute_ts),
  KEY idx_site_monitoring_pause_range (site_id, end_minute_ts, start_minute_ts),
  CONSTRAINT fk_site_monitoring_pause_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_capability (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  capability_key VARCHAR(64) NOT NULL,
  status VARCHAR(16) NOT NULL,
  message_code VARCHAR(64) NOT NULL DEFAULT '',
  message_params JSON NULL,
  checked_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_site_capability (site_id, capability_key),
  CONSTRAINT fk_site_capability_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS customer (
  id BIGINT NOT NULL AUTO_INCREMENT,
  name VARCHAR(128) NOT NULL,
  contact VARCHAR(255) NOT NULL DEFAULT '',
  remark VARCHAR(500) NOT NULL DEFAULT '',
  status VARCHAR(32) NOT NULL DEFAULT 'communicating',
  statistics_paused_at BIGINT NULL,
  statistics_backfill_status VARCHAR(16) NOT NULL DEFAULT 'none',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  KEY idx_customer_name (name),
  KEY idx_customer_status (status, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS account (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  customer_id BIGINT NOT NULL,
  remote_user_id BIGINT NOT NULL,
  remote_created_at BIGINT NOT NULL,
  username VARCHAR(255) NOT NULL,
  display_name VARCHAR(255) NOT NULL DEFAULT '',
  remote_group VARCHAR(128) NOT NULL DEFAULT '',
  remote_status INT NOT NULL DEFAULT 0,
  remote_state VARCHAR(32) NOT NULL DEFAULT 'normal',
  remote_missing_count INT NOT NULL DEFAULT 0,
  last_remote_seen_at BIGINT NULL,
  quota BIGINT NOT NULL DEFAULT 0,
  used_quota BIGINT NOT NULL DEFAULT 0,
  request_count BIGINT NOT NULL DEFAULT 0,
  managed_status VARCHAR(16) NOT NULL DEFAULT 'active',
  statistics_paused_at BIGINT NULL,
  statistics_backfill_status VARCHAR(16) NOT NULL DEFAULT 'pending',
  last_synced_at BIGINT NULL,
  remark VARCHAR(500) NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_account_site_remote_user (site_id, remote_user_id),
  KEY idx_account_customer_status (customer_id, managed_status),
  KEY idx_account_site_status (site_id, managed_status, remote_state),
  KEY idx_account_site_created (site_id, remote_created_at),
  KEY idx_account_remote_missing (site_id, remote_missing_count, last_remote_seen_at),
  KEY idx_account_username (username),
  CONSTRAINT fk_account_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT fk_account_customer FOREIGN KEY (customer_id) REFERENCES customer(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS collection_cursor (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  cursor_key VARCHAR(32) NOT NULL DEFAULT 'usage',
  last_complete_hour BIGINT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_collection_cursor (site_id, cursor_key),
  CONSTRAINT fk_collection_cursor_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS collection_run (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NULL,
  site_config_version INT NOT NULL DEFAULT 0,
  task_type VARCHAR(32) NOT NULL,
  target_type VARCHAR(16) NOT NULL DEFAULT 'site',
  target_id BIGINT NOT NULL DEFAULT 0,
  trigger_type VARCHAR(16) NOT NULL DEFAULT 'schedule',
  start_timestamp BIGINT NULL,
  end_timestamp BIGINT NULL,
  active_key VARCHAR(512) NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  fetched_rows BIGINT NOT NULL DEFAULT 0,
  written_rows BIGINT NOT NULL DEFAULT 0,
  retry_count INT NOT NULL DEFAULT 0,
  priority INT NOT NULL DEFAULT 0,
  next_attempt_at BIGINT NOT NULL,
  heartbeat_at BIGINT NULL,
  windows_initialized_at BIGINT NULL,
  total_windows INT NOT NULL DEFAULT 0,
  completed_windows INT NOT NULL DEFAULT 0,
  failed_windows INT NOT NULL DEFAULT 0,
  created_request_id VARCHAR(64) NOT NULL,
  last_request_id VARCHAR(64) NOT NULL,
  error_code VARCHAR(64) NOT NULL DEFAULT '',
  error_params JSON NULL,
  error_message TEXT NULL,
  started_at BIGINT NULL,
  finished_at BIGINT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_collection_run_active (active_key),
  KEY idx_collection_run_queue (status, next_attempt_at, priority, created_at),
  KEY idx_collection_run_materialize (windows_initialized_at, status, created_at),
  KEY idx_collection_run_site_range (site_id, task_type, start_timestamp),
  KEY idx_collection_run_target (target_type, target_id, created_at),
  CONSTRAINT fk_collection_run_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS collection_window (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  hour_ts BIGINT NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  fetched_rows BIGINT NOT NULL DEFAULT 0,
  source_hash CHAR(64) NOT NULL DEFAULT '',
  last_fact_run_id BIGINT NULL,
  verified_at BIGINT NULL,
  last_error_code VARCHAR(64) NOT NULL DEFAULT '',
  last_error_params JSON NULL,
  last_error_message TEXT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_collection_window (site_id, hour_ts),
  KEY idx_collection_window_status (status, hour_ts),
  KEY idx_collection_window_site_status (site_id, status, hour_ts),
  CONSTRAINT fk_collection_window_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT fk_collection_window_run FOREIGN KEY (last_fact_run_id) REFERENCES collection_run(id)
    ON UPDATE RESTRICT ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS collection_run_window (
  id BIGINT NOT NULL AUTO_INCREMENT,
  run_id BIGINT NOT NULL,
  site_id BIGINT NOT NULL,
  hour_ts BIGINT NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  attempt_count INT NOT NULL DEFAULT 0,
  next_retry_at BIGINT NULL,
  fetched_rows BIGINT NOT NULL DEFAULT 0,
  written_rows BIGINT NOT NULL DEFAULT 0,
  error_code VARCHAR(64) NOT NULL DEFAULT '',
  error_params JSON NULL,
  error_message TEXT NULL,
  started_at BIGINT NULL,
  finished_at BIGINT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_collection_run_window (run_id, site_id, hour_ts),
  KEY idx_collection_run_window_queue (run_id, status, next_retry_at, hour_ts),
  KEY idx_collection_run_window_site_hour (site_id, hour_ts, status),
  CONSTRAINT fk_collection_run_window_run FOREIGN KEY (run_id) REFERENCES collection_run(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT fk_collection_run_window_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS aggregation_bucket_lock (
  lock_key VARCHAR(64) NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (lock_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS usage_fact_hourly (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  remote_user_id BIGINT NOT NULL,
  username_snapshot VARCHAR(255) NOT NULL DEFAULT '',
  model_name VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  channel_id BIGINT NOT NULL DEFAULT 0,
  hour_ts BIGINT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  collected_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_usage_fact_hourly
    (site_id, remote_user_id, model_name, channel_id, hour_ts),
  KEY idx_usage_fact_hourly_site_time (site_id, hour_ts),
  KEY idx_usage_fact_hourly_user_time (site_id, remote_user_id, hour_ts),
  KEY idx_usage_fact_hourly_model_time (site_id, model_name, hour_ts),
  KEY idx_usage_fact_hourly_channel_time (site_id, channel_id, hour_ts),
  CONSTRAINT fk_usage_fact_hourly_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS usage_fact_daily (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  remote_user_id BIGINT NOT NULL,
  username_snapshot VARCHAR(255) NOT NULL DEFAULT '',
  model_name VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  channel_id BIGINT NOT NULL DEFAULT 0,
  date_key INT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  is_final TINYINT NOT NULL DEFAULT 0,
  last_calculated_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_usage_fact_daily
    (site_id, remote_user_id, model_name, channel_id, date_key),
  KEY idx_usage_fact_daily_site_date (site_id, date_key),
  KEY idx_usage_fact_daily_user_date (site_id, remote_user_id, date_key),
  KEY idx_usage_fact_daily_model_date (site_id, model_name, date_key),
  KEY idx_usage_fact_daily_channel_date (site_id, channel_id, date_key),
  CONSTRAINT fk_usage_fact_daily_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS account_stat_hourly (
  id BIGINT NOT NULL AUTO_INCREMENT,
  account_id BIGINT NOT NULL,
  hour_ts BIGINT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'complete',
  last_calculated_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_account_stat_hourly (account_id, hour_ts),
  KEY idx_account_stat_hourly_time (hour_ts, account_id),
  CONSTRAINT fk_account_stat_hourly_account FOREIGN KEY (account_id) REFERENCES account(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS account_stat_daily (
  id BIGINT NOT NULL AUTO_INCREMENT,
  account_id BIGINT NOT NULL,
  date_key INT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'complete',
  is_final TINYINT NOT NULL DEFAULT 0,
  last_calculated_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_account_stat_daily (account_id, date_key),
  KEY idx_account_stat_daily_date (date_key, account_id),
  CONSTRAINT fk_account_stat_daily_account FOREIGN KEY (account_id) REFERENCES account(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS customer_stat_hourly (
  id BIGINT NOT NULL AUTO_INCREMENT,
  customer_id BIGINT NOT NULL,
  site_id BIGINT NOT NULL,
  hour_ts BIGINT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  active_users BIGINT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'complete',
  last_calculated_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_customer_stat_hourly (customer_id, site_id, hour_ts),
  KEY idx_customer_stat_hourly_time (site_id, hour_ts, customer_id),
  CONSTRAINT fk_customer_stat_hourly_customer FOREIGN KEY (customer_id) REFERENCES customer(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT fk_customer_stat_hourly_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS customer_stat_daily (
  id BIGINT NOT NULL AUTO_INCREMENT,
  customer_id BIGINT NOT NULL,
  site_id BIGINT NOT NULL,
  date_key INT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  active_users BIGINT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'complete',
  is_final TINYINT NOT NULL DEFAULT 0,
  last_calculated_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_customer_stat_daily (customer_id, site_id, date_key),
  KEY idx_customer_stat_daily_date (site_id, date_key, customer_id),
  CONSTRAINT fk_customer_stat_daily_customer FOREIGN KEY (customer_id) REFERENCES customer(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT fk_customer_stat_daily_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_stat_hourly (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  hour_ts BIGINT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  active_users BIGINT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'complete',
  last_calculated_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_site_stat_hourly (site_id, hour_ts),
  KEY idx_site_stat_hourly_time (hour_ts, site_id),
  CONSTRAINT fk_site_stat_hourly_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_stat_daily (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  date_key INT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  active_users BIGINT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'complete',
  is_final TINYINT NOT NULL DEFAULT 0,
  last_calculated_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_site_stat_daily (site_id, date_key),
  KEY idx_site_stat_daily_date (date_key, site_id),
  CONSTRAINT fk_site_stat_daily_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS global_stat_hourly (
  id BIGINT NOT NULL AUTO_INCREMENT,
  hour_ts BIGINT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  active_users BIGINT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'complete',
  last_calculated_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_global_stat_hourly (hour_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS global_stat_daily (
  id BIGINT NOT NULL AUTO_INCREMENT,
  date_key INT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  active_users BIGINT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'complete',
  is_final TINYINT NOT NULL DEFAULT 0,
  last_calculated_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_global_stat_daily (date_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS model_stat_hourly (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  model_name VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  hour_ts BIGINT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  active_users BIGINT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'complete',
  last_calculated_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_model_stat_hourly (site_id, model_name, hour_ts),
  KEY idx_model_stat_hourly_time (site_id, hour_ts, model_name),
  CONSTRAINT fk_model_stat_hourly_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS model_stat_daily (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  model_name VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  date_key INT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  active_users BIGINT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'complete',
  is_final TINYINT NOT NULL DEFAULT 0,
  last_calculated_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_model_stat_daily (site_id, model_name, date_key),
  KEY idx_model_stat_daily_date (site_id, date_key, model_name),
  CONSTRAINT fk_model_stat_daily_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS channel_stat_hourly (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  channel_id BIGINT NOT NULL DEFAULT 0,
  hour_ts BIGINT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  active_users BIGINT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'complete',
  last_calculated_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_channel_stat_hourly (site_id, channel_id, hour_ts),
  KEY idx_channel_stat_hourly_time (site_id, hour_ts, channel_id),
  CONSTRAINT fk_channel_stat_hourly_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS channel_stat_daily (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  channel_id BIGINT NOT NULL DEFAULT 0,
  date_key INT NOT NULL,
  request_count BIGINT NOT NULL DEFAULT 0,
  quota BIGINT NOT NULL DEFAULT 0,
  token_used BIGINT NOT NULL DEFAULT 0,
  active_users BIGINT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'complete',
  is_final TINYINT NOT NULL DEFAULT 0,
  last_calculated_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_channel_stat_daily (site_id, channel_id, date_key),
  KEY idx_channel_stat_daily_date (site_id, date_key, channel_id),
  CONSTRAINT fk_channel_stat_daily_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_instance (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  node_name VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  hostname VARCHAR(255) NOT NULL DEFAULT '',
  is_master TINYINT NOT NULL DEFAULT 0,
  runtime_version VARCHAR(64) NOT NULL DEFAULT '',
  goos VARCHAR(32) NOT NULL DEFAULT '',
  goarch VARCHAR(32) NOT NULL DEFAULT '',
  upstream_status VARCHAR(16) NOT NULL DEFAULT 'unknown',
  upstream_stale_after_seconds INT NULL,
  current_status VARCHAR(16) NOT NULL DEFAULT 'unknown',
  first_seen_at BIGINT NOT NULL,
  started_at BIGINT NULL,
  last_seen_at BIGINT NULL,
  last_synced_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_site_instance_node (site_id, node_name),
  KEY idx_site_instance_status (site_id, current_status, last_seen_at),
  CONSTRAINT fk_site_instance_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_instance_status_minutely (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  node_name VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  minute_ts BIGINT NOT NULL,
  status VARCHAR(16) NOT NULL,
  cpu_percent DECIMAL(8,4) NULL,
  memory_percent DECIMAL(8,4) NULL,
  disk_used_percent DECIMAL(8,4) NULL,
  disk_total_bytes BIGINT NULL,
  disk_used_bytes BIGINT NULL,
  started_at BIGINT NULL,
  last_seen_at BIGINT NULL,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_instance_minutely (site_id, node_name, minute_ts),
  KEY idx_instance_minutely_time (site_id, minute_ts),
  CONSTRAINT fk_instance_minutely_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_instance_status_hourly (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  node_name VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  hour_ts BIGINT NOT NULL,
  cpu_max_percent DECIMAL(8,4) NULL,
  cpu_avg_percent DECIMAL(8,4) NULL,
  memory_max_percent DECIMAL(8,4) NULL,
  memory_avg_percent DECIMAL(8,4) NULL,
  disk_max_used_percent DECIMAL(8,4) NULL,
  disk_last_used_percent DECIMAL(8,4) NULL,
  online_samples INT NOT NULL DEFAULT 0,
  abnormal_samples INT NOT NULL DEFAULT 0,
  sample_count INT NOT NULL DEFAULT 0,
  expected_sample_count INT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'missing',
  last_calculated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_instance_hourly (site_id, node_name, hour_ts),
  KEY idx_instance_hourly_time (site_id, hour_ts),
  CONSTRAINT fk_instance_hourly_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_instance_status_daily (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  node_name VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  date_key INT NOT NULL,
  cpu_max_percent DECIMAL(8,4) NULL,
  cpu_avg_percent DECIMAL(8,4) NULL,
  memory_max_percent DECIMAL(8,4) NULL,
  memory_avg_percent DECIMAL(8,4) NULL,
  disk_max_used_percent DECIMAL(8,4) NULL,
  disk_last_used_percent DECIMAL(8,4) NULL,
  online_samples INT NOT NULL DEFAULT 0,
  abnormal_samples INT NOT NULL DEFAULT 0,
  sample_count INT NOT NULL DEFAULT 0,
  expected_sample_count INT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'missing',
  is_final TINYINT NOT NULL DEFAULT 0,
  last_calculated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_instance_daily (site_id, node_name, date_key),
  KEY idx_instance_daily_date (site_id, date_key),
  CONSTRAINT fk_instance_daily_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_status_minutely (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  minute_ts BIGINT NOT NULL,
  instance_count INT NOT NULL DEFAULT 0,
  online_instance_count INT NOT NULL DEFAULT 0,
  cpu_max_percent DECIMAL(8,4) NULL,
  cpu_avg_percent DECIMAL(8,4) NULL,
  memory_max_percent DECIMAL(8,4) NULL,
  memory_avg_percent DECIMAL(8,4) NULL,
  disk_max_used_percent DECIMAL(8,4) NULL,
  health_status VARCHAR(16) NOT NULL,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_site_minutely (site_id, minute_ts),
  KEY idx_site_minutely_time (minute_ts, site_id),
  CONSTRAINT fk_site_minutely_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_status_hourly (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  hour_ts BIGINT NOT NULL,
  instance_count_max INT NOT NULL DEFAULT 0,
  online_instance_count_min INT NOT NULL DEFAULT 0,
  cpu_max_percent DECIMAL(8,4) NULL,
  cpu_avg_percent DECIMAL(8,4) NULL,
  memory_max_percent DECIMAL(8,4) NULL,
  memory_avg_percent DECIMAL(8,4) NULL,
  disk_max_used_percent DECIMAL(8,4) NULL,
  abnormal_samples INT NOT NULL DEFAULT 0,
  sample_count INT NOT NULL DEFAULT 0,
  expected_sample_count INT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'missing',
  health_status VARCHAR(16) NOT NULL,
  last_calculated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_site_status_hourly (site_id, hour_ts),
  KEY idx_site_status_hourly_time (hour_ts, site_id),
  CONSTRAINT fk_site_status_hourly_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_status_daily (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  date_key INT NOT NULL,
  instance_count_max INT NOT NULL DEFAULT 0,
  online_instance_count_min INT NOT NULL DEFAULT 0,
  cpu_max_percent DECIMAL(8,4) NULL,
  cpu_avg_percent DECIMAL(8,4) NULL,
  memory_max_percent DECIMAL(8,4) NULL,
  memory_avg_percent DECIMAL(8,4) NULL,
  disk_max_used_percent DECIMAL(8,4) NULL,
  abnormal_samples INT NOT NULL DEFAULT 0,
  sample_count INT NOT NULL DEFAULT 0,
  expected_sample_count INT NOT NULL DEFAULT 0,
  data_status VARCHAR(16) NOT NULL DEFAULT 'missing',
  health_status VARCHAR(16) NOT NULL,
  is_final TINYINT NOT NULL DEFAULT 0,
  last_calculated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_site_status_daily (site_id, date_key),
  KEY idx_site_status_daily_date (date_key, site_id),
  CONSTRAINT fk_site_status_daily_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS alert_rule (
  id BIGINT NOT NULL AUTO_INCREMENT,
  rule_key VARCHAR(64) NOT NULL,
  name VARCHAR(128) NOT NULL,
  enabled TINYINT NOT NULL DEFAULT 1,
  level VARCHAR(16) NOT NULL,
  metric VARCHAR(64) NOT NULL,
  compare_operator VARCHAR(8) NOT NULL DEFAULT '>=',
  threshold_value DECIMAL(30,10) NULL,
  for_times INT NOT NULL DEFAULT 1,
  scope_type VARCHAR(16) NOT NULL DEFAULT 'global',
  scope_id BIGINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_alert_rule_scope (rule_key, level, scope_type, scope_id),
  KEY idx_alert_rule_enabled (enabled, metric, scope_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS alert_event (
  id BIGINT NOT NULL AUTO_INCREMENT,
  rule_id BIGINT NOT NULL,
  rule_key VARCHAR(64) NOT NULL,
  site_id BIGINT NULL,
  target_type VARCHAR(32) NOT NULL,
  target_key VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  active_key VARCHAR(384) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
  level VARCHAR(16) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  consecutive_count INT NOT NULL DEFAULT 1,
  current_value DECIMAL(30,10) NULL,
  threshold_value DECIMAL(30,10) NULL,
  message_code VARCHAR(64) NOT NULL,
  message_params JSON NULL,
  message VARCHAR(1000) NOT NULL,
  first_observed_at BIGINT NOT NULL,
  first_fired_at BIGINT NULL,
  last_fired_at BIGINT NULL,
  resolved_at BIGINT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_alert_event_active (active_key),
  KEY idx_alert_event_status_level (status, level, updated_at),
  KEY idx_alert_event_target (target_type, target_key, created_at),
  KEY idx_alert_event_site (site_id, status, updated_at),
  CONSTRAINT fk_alert_event_rule FOREIGN KEY (rule_id) REFERENCES alert_rule(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT fk_alert_event_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS alert_delivery (
  id BIGINT NOT NULL AUTO_INCREMENT,
  alert_event_id BIGINT NULL,
  event_type VARCHAR(16) NOT NULL,
  channel VARCHAR(16) NOT NULL DEFAULT 'dingtalk',
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  attempt_count INT NOT NULL DEFAULT 0,
  error_code VARCHAR(64) NOT NULL DEFAULT '',
  response_code INT NULL,
  response_message TEXT NULL,
  next_retry_at BIGINT NULL,
  sent_at BIGINT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_alert_delivery_event (alert_event_id, event_type, channel),
  KEY idx_alert_delivery_retry (status, next_retry_at),
  CONSTRAINT fk_alert_delivery_event FOREIGN KEY (alert_event_id) REFERENCES alert_event(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS export_job (
  id BIGINT NOT NULL AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  format VARCHAR(8) NOT NULL,
  statistics_type VARCHAR(32) NOT NULL,
  filters JSON NOT NULL,
  filter_hash CHAR(64) NOT NULL,
  active_key VARCHAR(192) NULL,
  rate_snapshot JSON NULL,
  data_snapshot_at BIGINT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  progress INT NOT NULL DEFAULT 0,
  attempt_count INT NOT NULL DEFAULT 0,
  next_attempt_at BIGINT NOT NULL,
  heartbeat_at BIGINT NULL,
  file_path VARCHAR(500) NULL,
  file_name VARCHAR(255) NULL,
  file_size BIGINT NOT NULL DEFAULT 0,
  row_count BIGINT NOT NULL DEFAULT 0,
  error_code VARCHAR(64) NOT NULL DEFAULT '',
  error_params JSON NULL,
  error_message TEXT NULL,
  expires_at BIGINT NULL,
  started_at BIGINT NULL,
  finished_at BIGINT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_export_job_active (active_key),
  KEY idx_export_job_user (user_id, created_at),
  KEY idx_export_job_queue (status, next_attempt_at, created_at),
  KEY idx_export_job_expiry (status, expires_at),
  CONSTRAINT fk_export_job_user FOREIGN KEY (user_id) REFERENCES platform_user(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS platform_setting (
  id BIGINT NOT NULL AUTO_INCREMENT,
  setting_key VARCHAR(128) NOT NULL,
  setting_value MEDIUMTEXT NOT NULL,
  value_type VARCHAR(16) NOT NULL,
  is_secret TINYINT NOT NULL DEFAULT 0,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_platform_setting_key (setting_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
