ALTER TABLE usage_fact_hourly
  ADD KEY idx_usage_fact_hourly_time_user (hour_ts, site_id, remote_user_id),
  ADD KEY idx_usage_fact_hourly_time_model_user (hour_ts, site_id, model_name, remote_user_id),
  ADD KEY idx_usage_fact_hourly_time_channel_user (hour_ts, site_id, channel_id, remote_user_id),
  ALGORITHM=INPLACE,
  LOCK=NONE;
