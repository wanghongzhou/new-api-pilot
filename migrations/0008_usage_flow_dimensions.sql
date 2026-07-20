ALTER TABLE usage_fact_hourly
  ADD COLUMN use_group VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '' AFTER channel_id,
  ADD COLUMN token_id BIGINT NOT NULL DEFAULT 0 AFTER use_group,
  ADD COLUMN token_name VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '' AFTER token_id,
  ADD COLUMN node_name VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '' AFTER token_name,
  DROP INDEX uk_usage_fact_hourly,
  ADD UNIQUE KEY uk_usage_fact_hourly
    (site_id, remote_user_id, model_name, channel_id, use_group, token_id, node_name, hour_ts),
  ADD KEY idx_usage_fact_hourly_group_time (site_id, use_group, hour_ts),
  ADD KEY idx_usage_fact_hourly_token_time (site_id, token_id, hour_ts),
  ADD KEY idx_usage_fact_hourly_node_time (site_id, node_name, hour_ts);

ALTER TABLE usage_fact_daily
  ADD COLUMN use_group VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '' AFTER channel_id,
  ADD COLUMN token_id BIGINT NOT NULL DEFAULT 0 AFTER use_group,
  ADD COLUMN token_name VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '' AFTER token_id,
  ADD COLUMN node_name VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '' AFTER token_name,
  DROP INDEX uk_usage_fact_daily,
  ADD UNIQUE KEY uk_usage_fact_daily
    (site_id, remote_user_id, model_name, channel_id, use_group, token_id, node_name, date_key),
  ADD KEY idx_usage_fact_daily_group_date (site_id, use_group, date_key),
  ADD KEY idx_usage_fact_daily_token_date (site_id, token_id, date_key),
  ADD KEY idx_usage_fact_daily_node_date (site_id, node_name, date_key);
