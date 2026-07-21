package model

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"

	"new-api-pilot/migrations"
)

func TestAuthoritativeSchemaContractsLoadAllMigrations(t *testing.T) {
	contracts, err := AuthoritativeSchemaContracts()
	if err != nil {
		t.Fatalf("AuthoritativeSchemaContracts() error = %v", err)
	}
	if len(contracts) != 69 {
		t.Fatalf("authoritative table count = %d, want 69", len(contracts))
	}
	for _, table := range []string{
		"schema_migration", "schema_migration_progress", "collection_run", "alert_delivery",
		"alert_evaluation_cursor", "export_job", "encryption_reencrypt_job", "encryption_reencrypt_item",
		"upstream_log_fact", "upstream_log_collection_state",
		"site_user_inventory", "site_user_inventory_hourly",
		"site_channel_inventory", "site_channel_inventory_hourly",
		"site_performance_metric_bucket", "site_performance_collection_state",
		"site_topup_order", "site_topup_collection_state", "site_redemption", "site_redemption_collection_state",
		"site_upstream_task", "site_upstream_task_collection_state",
		"site_group_catalog", "site_pricing_catalog", "pricing_group_collection_state",
		"site_system_task", "site_system_task_collection_state",
		"data_maintenance_state", "site_instance_lifecycle",
	} {
		contract, exists := contracts[table]
		if !exists || len(contract.Columns) == 0 || len(contract.Indexes) == 0 {
			t.Errorf("authoritative contract for %s = %#v", table, contract)
		}
	}
	if _, exists := contracts["collection_run"].Indexes["idx_collection_run_queue"]; !exists {
		t.Error("collection_run contract lost its migration indexes")
	}
	if _, exists := contracts["alert_delivery"].Indexes["idx_alert_delivery_claim"]; !exists {
		t.Error("alert_delivery contract is missing the 0003 claim index")
	}
	if _, exists := contracts["export_job"].Indexes["idx_export_job_claim"]; !exists {
		t.Error("export_job contract is missing the 0004 claim index")
	}
	for _, name := range []string{
		"idx_usage_fact_hourly_time_user",
		"idx_usage_fact_hourly_time_model_user",
		"idx_usage_fact_hourly_time_channel_user",
	} {
		if _, exists := contracts["usage_fact_hourly"].Indexes[name]; !exists {
			t.Errorf("usage_fact_hourly contract is missing the 0006 capacity index %s", name)
		}
	}
}

func TestPricingGroupCatalogSchemaContract(t *testing.T) {
	contracts, err := AuthoritativeSchemaContracts()
	if err != nil {
		t.Fatal(err)
	}
	group := contracts["site_group_catalog"]
	pricing := contracts["site_pricing_catalog"]
	state := contracts["pricing_group_collection_state"]
	if !reflect.DeepEqual(group.Indexes["uk_site_group_catalog_name"].Columns, []string{"site_id", "group_name"}) {
		t.Fatalf("group identity index=%#v", group.Indexes["uk_site_group_catalog_name"])
	}
	if !reflect.DeepEqual(pricing.Indexes["uk_site_pricing_catalog_identity"].Columns, []string{"site_id", "model_name", "vendor_key"}) {
		t.Fatalf("pricing identity index=%#v", pricing.Indexes["uk_site_pricing_catalog_identity"])
	}
	if !reflect.DeepEqual(state.Indexes["PRIMARY"].Columns, []string{"site_id", "resource_kind"}) {
		t.Fatalf("state identity index=%#v", state.Indexes["PRIMARY"])
	}
	for table, columns := range map[string][]string{
		"site_group_catalog":   {"ratio_decimal"},
		"site_pricing_catalog": {"model_ratio", "model_price", "completion_ratio", "cache_ratio", "create_cache_ratio", "image_ratio", "audio_ratio", "audio_completion_ratio"},
	} {
		byName := make(map[string]ColumnContract)
		for _, column := range contracts[table].Columns {
			byName[column.Name] = column
		}
		for _, name := range columns {
			if byName[name].ColumnType != "decimal(38,18)" {
				t.Fatalf("%s.%s=%#v", table, name, byName[name])
			}
		}
	}
}

func TestCustomerAmountSchemaContract(t *testing.T) {
	contracts, err := AuthoritativeSchemaContracts()
	if err != nil {
		t.Fatal(err)
	}
	columns := make(map[string]ColumnContract)
	for _, column := range contracts["customer"].Columns {
		columns[column.Name] = column
	}
	for _, name := range []string{"contract_amount", "payment_amount"} {
		column, exists := columns[name]
		if !exists || column.ColumnType != "decimal(38,10)" || column.IsNullable != "NO" ||
			!column.Default.Valid || column.Default.String != "0.0000000000" {
			t.Fatalf("customer.%s contract = %#v", name, column)
		}
	}
}

func TestParseCreateTableContractsIgnoresMultilineCheckExpressions(t *testing.T) {
	contracts, err := parseCreateTableContracts([]string{`CREATE TABLE IF NOT EXISTS check_fixture (
  id BIGINT NOT NULL,
  status VARCHAR(16) NOT NULL,
  open_status VARCHAR(16)
    GENERATED ALWAYS AS (CASE WHEN id >= 0 THEN status ELSE NULL END) STORED,
  CONSTRAINT chk_check_fixture CHECK (
    id >= 0 AND
    status IN ('ready', 'done')
  ),
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`})
	if err != nil {
		t.Fatal(err)
	}
	columns := contracts["check_fixture"].Columns
	if len(columns) != 3 || columns[0].Name != "id" || columns[1].Name != "status" ||
		columns[2].Name != "open_status" || columns[2].Extra != "STORED GENERATED" ||
		columns[2].GenerationExpression == "" {
		t.Fatalf("multiline CHECK expression parsed as columns: %#v", columns)
	}
}

func TestSystemTaskMonitoringSchemaContractExcludesRawPrivateFields(t *testing.T) {
	contracts, err := AuthoritativeSchemaContracts()
	if err != nil {
		t.Fatal(err)
	}
	contract := contracts["site_system_task"]
	if !reflect.DeepEqual(contract.Indexes["uk_site_system_task_remote"].Columns, []string{"site_id", "remote_id"}) {
		t.Fatalf("remote identity=%#v", contract.Indexes["uk_site_system_task_remote"])
	}
	for _, column := range contract.Columns {
		switch column.Name {
		case "active_key", "locked_by", "payload", "state", "result", "error", "raw_json":
			t.Fatalf("private raw column persisted: %s", column.Name)
		}
	}
	for _, required := range []string{"error_present", "error_code", "total", "processed", "progress", "deleted_count", "checked_channels", "unfinished_tasks"} {
		found := false
		for _, column := range contract.Columns {
			if column.Name == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing typed safe column %s", required)
		}
	}
	if !reflect.DeepEqual(contracts["site_system_task_collection_state"].Indexes["PRIMARY"].Columns, []string{"site_id", "resource_kind"}) {
		t.Fatalf("system task state identity=%#v", contracts["site_system_task_collection_state"].Indexes["PRIMARY"])
	}
}

func TestEncryptionReencryptMigrationChecksumIsFrozen(t *testing.T) {
	const currentChecksum = "d1723ccc1f824022f345f2935f8a75973a2f81824f8e0b2fc02db734f90e6d87"
	payload, err := migrations.Files.ReadFile("0005_encryption_reencrypt.sql")
	if err != nil {
		t.Fatalf("read 0005 migration: %v", err)
	}
	digest := sha256.Sum256(payload)
	if actual := hex.EncodeToString(digest[:]); actual != currentChecksum {
		t.Fatalf("0005 checksum = %s, want immutable published checksum %s; add a new migration instead of editing 0005", actual, currentChecksum)
	}
}

func TestUsageFactCapacityIndexMigrationChecksumIsFrozen(t *testing.T) {
	const currentChecksum = "f602607664a1d38789b6add38ce8ffa6688c0620a1e86e35585d55125a1b430a"
	payload, err := migrations.Files.ReadFile("0006_usage_fact_capacity_indexes.sql")
	if err != nil {
		t.Fatalf("read 0006 migration: %v", err)
	}
	digest := sha256.Sum256(payload)
	if actual := hex.EncodeToString(digest[:]); actual != currentChecksum {
		t.Fatalf("0006 checksum = %s, want immutable published checksum %s; add a new migration instead of editing 0006", actual, currentChecksum)
	}
}
