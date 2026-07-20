CREATE TABLE IF NOT EXISTS upstream_log_fact (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  config_version INT NOT NULL,
  upstream_log_key CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  upstream_log_id BIGINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  type INT NOT NULL,
  remote_user_id BIGINT NOT NULL DEFAULT 0,
  username VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  model_name VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  token_id BIGINT NOT NULL DEFAULT 0,
  token_name VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  channel_id BIGINT NOT NULL DEFAULT 0,
  use_group VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  request_id VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  upstream_request_id VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  quota BIGINT NOT NULL DEFAULT 0,
  prompt_tokens BIGINT NOT NULL DEFAULT 0,
  completion_tokens BIGINT NOT NULL DEFAULT 0,
  use_time_seconds BIGINT NOT NULL DEFAULT 0,
  is_stream TINYINT(1) NOT NULL DEFAULT 0,
  content_redacted VARCHAR(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  ip VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  collected_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_upstream_log_fact_site_key (site_id, config_version, upstream_log_key),
  KEY idx_upstream_log_fact_site_time (site_id, created_at, id),
  KEY idx_upstream_log_fact_type_time (type, created_at, id),
  KEY idx_upstream_log_fact_user_time (site_id, remote_user_id, created_at),
  KEY idx_upstream_log_fact_model_time (site_id, model_name, created_at),
  KEY idx_upstream_log_fact_token_time (site_id, token_id, created_at),
  KEY idx_upstream_log_fact_channel_time (site_id, channel_id, created_at),
  KEY idx_upstream_log_fact_group_time (site_id, use_group, created_at),
  KEY idx_upstream_log_fact_request (site_id, request_id),
  KEY idx_upstream_log_fact_upstream_request (site_id, upstream_request_id),
  CONSTRAINT fk_upstream_log_fact_site FOREIGN KEY (site_id) REFERENCES site(id) ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS upstream_log_collection_state (
  site_id BIGINT NOT NULL,
  config_version INT NOT NULL,
  status VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  window_start BIGINT NOT NULL,
  window_end BIGINT NOT NULL,
  last_success_at BIGINT NULL,
  last_error_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  last_error_params JSON NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (site_id),
  KEY idx_upstream_log_collection_state_status (status, updated_at),
  CONSTRAINT fk_upstream_log_collection_state_site FOREIGN KEY (site_id) REFERENCES site(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT chk_upstream_log_collection_state_status CHECK (status IN ('pending','complete','partial','unavailable','disabled')),
  CONSTRAINT chk_upstream_log_collection_state_window CHECK (window_start >= 0 AND window_end >= window_start)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

INSERT INTO platform_setting (setting_key, setting_value, value_type, is_secret, updated_at)
VALUES ('logs.retention_days', '30', 'int', 0, UNIX_TIMESTAMP())
ON DUPLICATE KEY UPDATE setting_key = VALUES(setting_key);
