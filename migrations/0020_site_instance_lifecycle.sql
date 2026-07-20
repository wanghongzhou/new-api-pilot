CREATE TABLE IF NOT EXISTS site_instance_lifecycle (
  id BIGINT NOT NULL AUTO_INCREMENT,
  site_id BIGINT NOT NULL,
  node_name VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  start_minute_ts BIGINT NOT NULL,
  end_minute_ts BIGINT NULL,
  evidence_status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'known',
  open_node_name VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin
    GENERATED ALWAYS AS (CASE WHEN end_minute_ts IS NULL THEN node_name ELSE NULL END) STORED,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_site_instance_lifecycle_start (site_id, node_name, start_minute_ts),
  UNIQUE KEY uk_site_instance_lifecycle_open (site_id, open_node_name),
  KEY idx_site_instance_lifecycle_range (site_id, start_minute_ts, end_minute_ts, node_name),
  CONSTRAINT fk_site_instance_lifecycle_site FOREIGN KEY (site_id) REFERENCES site(id)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT chk_site_instance_lifecycle_range CHECK (
    start_minute_ts > 0 AND MOD(start_minute_ts, 60) = 0 AND
    (end_minute_ts IS NULL OR (end_minute_ts >= start_minute_ts AND MOD(end_minute_ts, 60) = 0)) AND
    evidence_status IN ('known','legacy_unknown') AND created_at > 0 AND updated_at >= created_at
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO site_instance_lifecycle
  (site_id, node_name, start_minute_ts, end_minute_ts, evidence_status, created_at, updated_at)
SELECT site_id, node_name, first_seen_at - MOD(first_seen_at, 60),
       CASE WHEN retired_at IS NULL THEN NULL ELSE retired_at - MOD(retired_at, 60) END,
       'legacy_unknown', created_at, updated_at
FROM site_instance
ON DUPLICATE KEY UPDATE updated_at = GREATEST(site_instance_lifecycle.updated_at, VALUES(updated_at));
