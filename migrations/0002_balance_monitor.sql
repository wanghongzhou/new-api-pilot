CREATE TABLE IF NOT EXISTS balance_monitor_record (
  id bigint NOT NULL AUTO_INCREMENT,
  source_id varchar(64) NOT NULL,
  record_id varchar(64) NOT NULL,
  revision bigint NOT NULL,
  kind varchar(16) NOT NULL,
  account_id varchar(64) NOT NULL,
  site_id varchar(64) NOT NULL,
  day varchar(10) NOT NULL,
  sampled_at bigint NOT NULL,
  received_at bigint NOT NULL,
  consumption_yuan decimal(30,10) DEFAULT NULL,
  payload json NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_balance_record (source_id,record_id),
  KEY ix_balance_day (kind,day,account_id),
  KEY ix_balance_site (source_id,kind,site_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
