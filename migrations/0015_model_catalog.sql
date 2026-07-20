CREATE TABLE IF NOT EXISTS site_model_meta (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  remote_id BIGINT NOT NULL,
  model_name VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  description TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  icon VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  tags VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  vendor_id BIGINT NOT NULL,
  remote_status INT NOT NULL,
  sync_official INT NOT NULL,
  name_rule INT NOT NULL,
  remote_created_time BIGINT NOT NULL,
  remote_updated_time BIGINT NOT NULL,
  source_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  config_version INT NOT NULL,
  collected_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_site_model_remote (site_id, remote_id),
  KEY idx_site_model_name (site_id, model_name, name_rule),
  KEY idx_site_model_vendor (vendor_id, remote_status, site_id),
  CONSTRAINT fk_site_model_site FOREIGN KEY (site_id) REFERENCES site(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT chk_site_model_values CHECK (remote_id > 0 AND vendor_id >= 0 AND name_rule BETWEEN 0 AND 3 AND remote_created_time >= 0 AND remote_updated_time >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS site_model_meta_collection_state (
  site_id BIGINT NOT NULL,
  last_success_at BIGINT NULL,
  last_failure_at BIGINT NULL,
  last_error_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  observed_count BIGINT NOT NULL DEFAULT 0,
  config_version INT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (site_id),
  CONSTRAINT fk_site_model_state_site FOREIGN KEY (site_id) REFERENCES site(id) ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS site_channel_model_mapping (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  remote_channel_id BIGINT NOT NULL,
  model_name VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  remote_group VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  config_version INT NOT NULL,
  collected_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_site_channel_model_group (site_id, remote_channel_id, model_name, remote_group),
  KEY idx_site_channel_model_name (site_id, model_name),
  CONSTRAINT fk_site_channel_model_site FOREIGN KEY (site_id) REFERENCES site(id) ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
