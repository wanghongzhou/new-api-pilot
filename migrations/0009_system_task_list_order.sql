SET @system_task_list_order_exists = (
  SELECT COUNT(*)
  FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'site_system_task'
    AND index_name = 'idx_site_system_task_list_order'
);
SET @system_task_list_order_sql = IF(
  @system_task_list_order_exists = 0,
  'ALTER TABLE site_system_task ADD KEY idx_site_system_task_list_order (site_id ASC, remote_id DESC)',
  'SELECT 1'
);
PREPARE system_task_list_order_statement FROM @system_task_list_order_sql;
EXECUTE system_task_list_order_statement;
DEALLOCATE PREPARE system_task_list_order_statement;
