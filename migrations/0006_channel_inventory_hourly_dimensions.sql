ALTER TABLE site_channel_inventory_hourly
  ADD COLUMN remote_type INT NOT NULL DEFAULT -1 AFTER site_id,
  ADD COLUMN remote_status INT NOT NULL DEFAULT -1 AFTER remote_type,
  ADD COLUMN remote_group VARCHAR(128) NOT NULL DEFAULT '' AFTER remote_status,
  ADD COLUMN tag VARCHAR(255) NOT NULL DEFAULT '' AFTER remote_group,
  ADD COLUMN dimensions_available TINYINT NOT NULL DEFAULT 0 AFTER tag,
  DROP INDEX uk_site_channel_inventory_hourly,
  ADD UNIQUE KEY uk_site_channel_inventory_hourly (site_id,remote_type,remote_status,remote_group,tag,hour_ts),
  ADD KEY idx_site_channel_inventory_hourly_filters (site_id,hour_ts,dimensions_available,remote_type,remote_status);
