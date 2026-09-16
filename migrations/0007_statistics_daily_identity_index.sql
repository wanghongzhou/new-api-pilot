ALTER TABLE usage_fact_daily
  ADD INDEX idx_usage_fact_daily_date_user (date_key, site_id, remote_user_id),
  ALGORITHM=INPLACE,
  LOCK=NONE;
