CREATE TABLE IF NOT EXISTS encryption_reencrypt_job (
  id BIGINT NOT NULL AUTO_INCREMENT,
  old_key_id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  new_key_id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  active_key VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NULL,
  state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'staging',
  inventory_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  total_items BIGINT NOT NULL DEFAULT 0,
  staged_items BIGINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_encryption_reencrypt_active (active_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS encryption_reencrypt_item (
  job_id BIGINT NOT NULL,
  row_type VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  row_id BIGINT NOT NULL,
  aad_identity VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  source_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  new_ciphertext MEDIUMTEXT NOT NULL,
  needs_update TINYINT NOT NULL DEFAULT 1,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (job_id, row_type, row_id),
  KEY idx_encryption_reencrypt_item_job (job_id, row_type, row_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
