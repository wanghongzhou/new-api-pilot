UPDATE collection_window AS window_row
LEFT JOIN (
  SELECT site_id, hour_ts, COUNT(*) AS fact_rows
  FROM usage_fact_hourly
  GROUP BY site_id, hour_ts
) AS facts
  ON facts.site_id = window_row.site_id AND facts.hour_ts = window_row.hour_ts
SET window_row.last_error_code = 'DATA_VALIDATION_MISMATCH',
    window_row.last_error_params = NULL,
    window_row.last_error_message = NULL,
    window_row.verified_at = NULL,
    window_row.updated_at = GREATEST(window_row.updated_at, UNIX_TIMESTAMP())
WHERE window_row.status = 'missing'
  AND window_row.fact_rows <> COALESCE(facts.fact_rows, 0);
