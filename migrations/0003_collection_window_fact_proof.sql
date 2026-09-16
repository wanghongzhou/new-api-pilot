ALTER TABLE collection_window
  ADD COLUMN fact_rows bigint NOT NULL DEFAULT 0 AFTER fetched_rows;

UPDATE collection_window AS window_row
LEFT JOIN (
  SELECT site_id, hour_ts, COUNT(*) AS fact_rows
  FROM usage_fact_hourly
  GROUP BY site_id, hour_ts
) AS facts
  ON facts.site_id = window_row.site_id AND facts.hour_ts = window_row.hour_ts
LEFT JOIN collection_run_window AS run_window
  ON run_window.run_id = window_row.last_fact_run_id
 AND run_window.site_id = window_row.site_id
 AND run_window.hour_ts = window_row.hour_ts
SET window_row.status = CASE
      WHEN window_row.status = 'complete'
       AND run_window.written_rows IS NOT NULL
       AND run_window.written_rows <> COALESCE(facts.fact_rows, 0)
      THEN 'missing'
      ELSE window_row.status
    END,
    window_row.last_error_code = CASE
      WHEN window_row.status = 'complete'
       AND run_window.written_rows IS NOT NULL
       AND run_window.written_rows <> COALESCE(facts.fact_rows, 0)
      THEN 'DATA_VALIDATION_MISMATCH'
      ELSE window_row.last_error_code
    END,
    window_row.verified_at = CASE
      WHEN window_row.status = 'complete'
       AND run_window.written_rows IS NOT NULL
       AND run_window.written_rows <> COALESCE(facts.fact_rows, 0)
      THEN NULL
      ELSE window_row.verified_at
    END,
    window_row.fact_rows = COALESCE(run_window.written_rows, facts.fact_rows, 0);
