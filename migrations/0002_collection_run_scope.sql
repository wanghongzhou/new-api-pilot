ALTER TABLE collection_run
  ADD COLUMN scope JSON NULL AFTER end_timestamp;

UPDATE collection_run
SET scope = CASE
  WHEN task_type = 'usage_backfill' THEN JSON_OBJECT('only_missing', TRUE)
  ELSE JSON_OBJECT()
END
WHERE scope IS NULL;

ALTER TABLE collection_run
  MODIFY COLUMN scope JSON NOT NULL AFTER end_timestamp;
