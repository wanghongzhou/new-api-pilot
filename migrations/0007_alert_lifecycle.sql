ALTER TABLE alert_event
  ADD COLUMN resolution_reason VARCHAR(32) NULL AFTER resolved_at;

ALTER TABLE site_instance
  ADD COLUMN retired_at BIGINT NULL AFTER updated_at,
  ADD KEY idx_site_instance_active (site_id, retired_at, node_name);
