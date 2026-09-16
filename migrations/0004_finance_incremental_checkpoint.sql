ALTER TABLE site_topup_collection_state
  ADD COLUMN last_full_success_at bigint DEFAULT NULL AFTER last_success_at;

ALTER TABLE site_redemption_collection_state
  ADD COLUMN last_full_success_at bigint DEFAULT NULL AFTER last_success_at;
