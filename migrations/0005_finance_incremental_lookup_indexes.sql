ALTER TABLE site_topup_order
  DROP INDEX idx_site_topup_order_site_status,
  ADD INDEX idx_site_topup_order_site_status (site_id, remote_status, remote_state, remote_id);

ALTER TABLE site_redemption
  DROP INDEX idx_site_redemption_site_status,
  ADD INDEX idx_site_redemption_site_status (site_id, remote_status, remote_state, remote_id);
