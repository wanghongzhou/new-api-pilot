ALTER TABLE site_group_catalog
  ADD COLUMN topup_ratio_decimal DECIMAL(38,18) NULL AFTER ratio_decimal,
  ADD COLUMN user_selectable TINYINT(1) NOT NULL DEFAULT '0' AFTER description,
  ADD COLUMN default_use_auto_group TINYINT(1) NOT NULL DEFAULT '0' AFTER user_selectable,
  ADD COLUMN auto_priority INT NULL AFTER default_use_auto_group,
  ADD COLUMN outgoing_overrides_json JSON NULL AFTER auto_priority,
  ADD COLUMN incoming_overrides_json JSON NULL AFTER outgoing_overrides_json,
  ADD COLUMN visible_to_groups_json JSON NULL AFTER incoming_overrides_json,
  ADD COLUMN hidden_from_groups_json JSON NULL AFTER visible_to_groups_json;

UPDATE site_group_catalog
SET outgoing_overrides_json = JSON_OBJECT(),
    incoming_overrides_json = JSON_OBJECT(),
    visible_to_groups_json = JSON_OBJECT(),
    hidden_from_groups_json = JSON_ARRAY()
WHERE outgoing_overrides_json IS NULL
   OR incoming_overrides_json IS NULL
   OR visible_to_groups_json IS NULL
   OR hidden_from_groups_json IS NULL;

ALTER TABLE site_group_catalog
  MODIFY COLUMN outgoing_overrides_json JSON NOT NULL,
  MODIFY COLUMN incoming_overrides_json JSON NOT NULL,
  MODIFY COLUMN visible_to_groups_json JSON NOT NULL,
  MODIFY COLUMN hidden_from_groups_json JSON NOT NULL;

ALTER TABLE site_pricing_catalog
  ADD COLUMN billing_mode VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'token' AFTER pricing_version,
  ADD COLUMN billing_expr MEDIUMTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER billing_mode,
  ADD COLUMN pricing_source VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'token_default' AFTER billing_expr,
  ADD COLUMN ability_available TINYINT(1) NOT NULL DEFAULT '0' AFTER pricing_source;

UPDATE site_pricing_catalog
SET billing_expr = ''
WHERE billing_expr IS NULL;

ALTER TABLE site_pricing_catalog
  MODIFY COLUMN billing_expr MEDIUMTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL;

DELETE stale
FROM site_pricing_catalog stale
JOIN site_pricing_catalog preferred
  ON preferred.site_id = stale.site_id
 AND preferred.model_name = stale.model_name
 AND (
   (stale.remote_state <> 'normal' AND preferred.remote_state = 'normal')
   OR (
     (stale.remote_state = 'normal') = (preferred.remote_state = 'normal')
     AND (
       stale.updated_at < preferred.updated_at
       OR (stale.updated_at = preferred.updated_at AND stale.id < preferred.id)
     )
   )
 );

ALTER TABLE site_pricing_catalog
  DROP INDEX uk_site_pricing_catalog_identity,
  ADD UNIQUE KEY uk_site_pricing_catalog_identity (site_id,model_name);
