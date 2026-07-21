package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"new-api-pilot/migrations"
)

type SchemaQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type TableContract struct {
	Columns     []ColumnContract
	Indexes     map[string]IndexContract
	ForeignKeys map[string]ForeignKeyContract
	Engine      string
	Charset     string
	Collation   string
}

type ColumnContract struct {
	Name                 string
	ColumnType           string
	IsNullable           string
	Default              sql.NullString
	Extra                string
	GenerationExpression string
	CharacterSet         sql.NullString
	Collation            sql.NullString
}

type IndexContract struct {
	Unique  bool
	Columns []string
}

type ForeignKeyContract struct {
	Columns           []string
	ReferencedTable   string
	ReferencedColumns []string
	UpdateRule        string
	DeleteRule        string
}

type SchemaVerificationSummary struct {
	Tables      int
	Columns     int
	Indexes     int
	ForeignKeys int
}

var (
	schemaCreateTablePattern = regexp.MustCompile(`(?s)^CREATE TABLE IF NOT EXISTS ([a-z0-9_]+) \((.*)\) ENGINE=([A-Za-z0-9]+) DEFAULT CHARSET=([a-z0-9_]+) COLLATE=([a-z0-9_]+)$`)
	schemaColumnNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	schemaColumnTypePattern  = regexp.MustCompile(`^(?:bigint|char\([0-9]+\)|decimal\([0-9]+,[0-9]+\)|int|json|mediumtext|text|tinyint(?:\(1\))?|varchar\([0-9]+\))$`)
	schemaColumnDefault      = regexp.MustCompile(`\bDEFAULT\s+('(?:''|[^'])*'|[^\s,]+)`)
	schemaColumnCharset      = regexp.MustCompile(`\bCHARACTER SET\s+([a-z0-9_]+)`)
	schemaColumnCollation    = regexp.MustCompile(`\bCOLLATE\s+([a-z0-9_]+)`)
	schemaColumnGenerated    = regexp.MustCompile(`(?i)\bGENERATED ALWAYS AS\s*\((.*)\)\s*(?:VIRTUAL|STORED)?`)
	schemaPrimaryIndex       = regexp.MustCompile(`(?s)PRIMARY KEY\s*\(([^)]+)\)`)
	schemaUniqueIndex        = regexp.MustCompile(`(?s)UNIQUE KEY\s+([a-z0-9_]+)\s*\(([^)]+)\)`)
	schemaSecondaryIndex     = regexp.MustCompile(`(?m)^\s*KEY\s+([a-z0-9_]+)\s*\(([^)]+)\)`)
	schemaForeignKey         = regexp.MustCompile(`(?s)CONSTRAINT\s+([a-z0-9_]+)\s+FOREIGN KEY\s*\(([^)]+)\)\s+REFERENCES\s+([a-z0-9_]+)\s*\(([^)]+)\)\s+ON UPDATE\s+(RESTRICT|CASCADE|SET NULL|NO ACTION)\s+ON DELETE\s+(RESTRICT|CASCADE|SET NULL|NO ACTION)`)
)

func AuthoritativeSchemaContracts() (map[string]TableContract, error) {
	initialStatements, err := readMigrationStatements("0001_initial_schema.sql")
	if err != nil {
		return nil, err
	}
	contracts, err := parseCreateTableContracts(initialStatements)
	if err != nil {
		return nil, err
	}
	applySchemaCustomerAmountsContract(contracts)
	applySchemaCollectionRunScopeContract(contracts)
	applySchemaAlertReliabilityContract(contracts)
	applySchemaExportClaimLeaseContract(contracts)
	applySchemaUsageFactCapacityIndexContract(contracts)
	applySchemaAlertLifecycleContract(contracts)
	applySchemaUsageFlowDimensionsContract(contracts)

	logStatements, err := readMigrationStatements("0009_upstream_log_facts.sql")
	if err != nil {
		return nil, err
	}
	if len(logStatements) < 2 {
		return nil, fmt.Errorf("0009 log migration is incomplete")
	}
	logContracts, err := parseCreateTableContracts(logStatements[:2])
	if err != nil {
		return nil, err
	}
	for name, contract := range logContracts {
		if _, exists := contracts[name]; exists {
			return nil, fmt.Errorf("migration schema contract duplicates table %s", name)
		}
		contracts[name] = contract
	}

	inventoryStatements, err := readMigrationStatements("0010_site_user_inventory.sql")
	if err != nil {
		return nil, err
	}
	if len(inventoryStatements) != 2 {
		return nil, fmt.Errorf("0010 inventory migration is incomplete")
	}
	inventoryContracts, err := parseCreateTableContracts(inventoryStatements)
	if err != nil {
		return nil, err
	}
	for name, contract := range inventoryContracts {
		if _, exists := contracts[name]; exists {
			return nil, fmt.Errorf("migration schema contract duplicates table %s", name)
		}
		contracts[name] = contract
	}

	channelInventoryStatements, err := readMigrationStatements("0011_site_channel_inventory.sql")
	if err != nil {
		return nil, err
	}
	if len(channelInventoryStatements) != 2 {
		return nil, fmt.Errorf("0011 channel inventory migration is incomplete")
	}
	channelInventoryContracts, err := parseCreateTableContracts(channelInventoryStatements)
	if err != nil {
		return nil, err
	}
	for name, contract := range channelInventoryContracts {
		if _, exists := contracts[name]; exists {
			return nil, fmt.Errorf("migration schema contract duplicates table %s", name)
		}
		contracts[name] = contract
	}
	performanceStatements, err := readMigrationStatements("0012_performance_history.sql")
	if err != nil {
		return nil, err
	}
	if len(performanceStatements) != 2 {
		return nil, fmt.Errorf("0012 performance migration is incomplete")
	}
	performanceContracts, err := parseCreateTableContracts(performanceStatements)
	if err != nil {
		return nil, err
	}
	for name, contract := range performanceContracts {
		if _, exists := contracts[name]; exists {
			return nil, fmt.Errorf("migration schema contract duplicates table %s", name)
		}
		contracts[name] = contract
	}
	financeStatements, err := readMigrationStatements("0013_finance_operations.sql")
	if err != nil {
		return nil, err
	}
	if len(financeStatements) != 4 {
		return nil, fmt.Errorf("0013 finance operations migration is incomplete")
	}
	financeContracts, err := parseCreateTableContracts(financeStatements)
	if err != nil {
		return nil, err
	}
	for name, contract := range financeContracts {
		if _, exists := contracts[name]; exists {
			return nil, fmt.Errorf("migration schema contract duplicates table %s", name)
		}
		contracts[name] = contract
	}
	taskStatements, err := readMigrationStatements("0014_upstream_tasks.sql")
	if err != nil {
		return nil, err
	}
	if len(taskStatements) != 2 {
		return nil, fmt.Errorf("0014 upstream tasks migration is incomplete")
	}
	taskContracts, err := parseCreateTableContracts(taskStatements)
	if err != nil {
		return nil, err
	}
	for name, contract := range taskContracts {
		if _, exists := contracts[name]; exists {
			return nil, fmt.Errorf("migration schema contract duplicates table %s", name)
		}
		contracts[name] = contract
	}
	modelStatements, err := readMigrationStatements("0015_model_catalog.sql")
	if err != nil {
		return nil, err
	}
	if len(modelStatements) != 3 {
		return nil, fmt.Errorf("0015 model catalog migration is incomplete")
	}
	modelContracts, err := parseCreateTableContracts(modelStatements)
	if err != nil {
		return nil, err
	}
	for name, contract := range modelContracts {
		if _, exists := contracts[name]; exists {
			return nil, fmt.Errorf("migration schema contract duplicates table %s", name)
		}
		contracts[name] = contract
	}
	planStatements, err := readMigrationStatements("0016_subscription_plans.sql")
	if err != nil {
		return nil, err
	}
	if len(planStatements) != 2 {
		return nil, fmt.Errorf("0016 subscription plans migration is incomplete")
	}
	planContracts, err := parseCreateTableContracts(planStatements)
	if err != nil {
		return nil, err
	}
	for name, contract := range planContracts {
		if _, exists := contracts[name]; exists {
			return nil, fmt.Errorf("migration schema contract duplicates table %s", name)
		}
		contracts[name] = contract
	}
	pricingGroupStatements, err := readMigrationStatements("0017_pricing_group_catalog.sql")
	if err != nil {
		return nil, err
	}
	if len(pricingGroupStatements) != 3 {
		return nil, fmt.Errorf("0017 pricing group catalog migration is incomplete")
	}
	pricingGroupContracts, err := parseCreateTableContracts(pricingGroupStatements)
	if err != nil {
		return nil, err
	}
	for name, contract := range pricingGroupContracts {
		if _, exists := contracts[name]; exists {
			return nil, fmt.Errorf("migration schema contract duplicates table %s", name)
		}
		contracts[name] = contract
	}
	systemTaskStatements, err := readMigrationStatements("0018_system_task_monitoring.sql")
	if err != nil {
		return nil, err
	}
	if len(systemTaskStatements) != 2 {
		return nil, fmt.Errorf("0018 system task monitoring migration is incomplete")
	}
	systemTaskContracts, err := parseCreateTableContracts(systemTaskStatements)
	if err != nil {
		return nil, err
	}
	for name, contract := range systemTaskContracts {
		if _, exists := contracts[name]; exists {
			return nil, fmt.Errorf("migration schema contract duplicates table %s", name)
		}
		contracts[name] = contract
	}
	maintenanceStatements, err := readMigrationStatements("0019_data_maintenance.sql")
	if err != nil {
		return nil, err
	}
	if len(maintenanceStatements) != 1 {
		return nil, fmt.Errorf("0019 data maintenance migration is incomplete")
	}
	maintenanceContracts, err := parseCreateTableContracts(maintenanceStatements)
	if err != nil {
		return nil, err
	}
	for name, contract := range maintenanceContracts {
		if _, exists := contracts[name]; exists {
			return nil, fmt.Errorf("migration schema contract duplicates table %s", name)
		}
		contracts[name] = contract
	}
	lifecycleStatements, err := readMigrationStatements("0020_site_instance_lifecycle.sql")
	if err != nil {
		return nil, err
	}
	if len(lifecycleStatements) != 2 {
		return nil, fmt.Errorf("0020 site instance lifecycle migration is incomplete")
	}
	lifecycleContracts, err := parseCreateTableContracts(lifecycleStatements[:1])
	if err != nil {
		return nil, err
	}
	for name, contract := range lifecycleContracts {
		if _, exists := contracts[name]; exists {
			return nil, fmt.Errorf("migration schema contract duplicates table %s", name)
		}
		contracts[name] = contract
	}

	reencryptStatements, err := readMigrationStatements("0005_encryption_reencrypt.sql")
	if err != nil {
		return nil, err
	}
	reencryptContracts, err := parseCreateTableContracts(reencryptStatements)
	if err != nil {
		return nil, err
	}
	for name, contract := range reencryptContracts {
		if _, exists := contracts[name]; exists {
			return nil, fmt.Errorf("migration schema contract duplicates table %s", name)
		}
		contracts[name] = contract
	}
	contracts["schema_migration_progress"] = migrationProgressTableContract()
	return contracts, nil
}

func applySchemaCustomerAmountsContract(contracts map[string]TableContract) {
	contract, ok := contracts["customer"]
	if !ok {
		return
	}
	amounts := []ColumnContract{
		{Name: "contract_amount", ColumnType: "decimal(38,10)", IsNullable: "NO", Default: sql.NullString{String: "0.0000000000", Valid: true}},
		{Name: "payment_amount", ColumnType: "decimal(38,10)", IsNullable: "NO", Default: sql.NullString{String: "0.0000000000", Valid: true}},
	}
	insertAt := len(contract.Columns)
	for index, column := range contract.Columns {
		if column.Name == "remark" {
			insertAt = index + 1
			break
		}
	}
	contract.Columns = append(contract.Columns, amounts...)
	copy(contract.Columns[insertAt+len(amounts):], contract.Columns[insertAt:len(contract.Columns)-len(amounts)])
	copy(contract.Columns[insertAt:], amounts)
	contracts["customer"] = contract
}

func applySchemaUsageFlowDimensionsContract(contracts map[string]TableContract) {
	for _, spec := range []struct {
		table      string
		timeColumn string
		suffix     string
	}{
		{table: "usage_fact_hourly", timeColumn: "hour_ts", suffix: "time"},
		{table: "usage_fact_daily", timeColumn: "date_key", suffix: "date"},
	} {
		contract := contracts[spec.table]
		columns := make([]ColumnContract, 0, len(contract.Columns)+4)
		for _, column := range contract.Columns {
			columns = append(columns, column)
			if column.Name == "channel_id" {
				columns = append(columns,
					usageFlowStringColumn("use_group", "varchar(128)"),
					ColumnContract{Name: "token_id", ColumnType: "bigint", IsNullable: "NO", Default: sql.NullString{String: "0", Valid: true}},
					usageFlowStringColumn("token_name", "varchar(255)"),
					usageFlowStringColumn("node_name", "varchar(128)"),
				)
			}
		}
		contract.Columns = columns
		contract.Indexes["uk_"+spec.table] = IndexContract{Unique: true, Columns: []string{
			"site_id", "remote_user_id", "model_name", "channel_id", "use_group", "token_id", "node_name", spec.timeColumn,
		}}
		contract.Indexes["idx_"+spec.table+"_group_"+spec.suffix] = IndexContract{Columns: []string{"site_id", "use_group", spec.timeColumn}}
		contract.Indexes["idx_"+spec.table+"_token_"+spec.suffix] = IndexContract{Columns: []string{"site_id", "token_id", spec.timeColumn}}
		contract.Indexes["idx_"+spec.table+"_node_"+spec.suffix] = IndexContract{Columns: []string{"site_id", "node_name", spec.timeColumn}}
		contracts[spec.table] = contract
	}
}

func usageFlowStringColumn(name, columnType string) ColumnContract {
	return ColumnContract{
		Name: name, ColumnType: columnType, IsNullable: "NO", Default: sql.NullString{String: "", Valid: true},
		CharacterSet: sql.NullString{String: "utf8mb4", Valid: true},
		Collation:    sql.NullString{String: "utf8mb4_bin", Valid: true},
	}
}

func VerifyAuthoritativeSchema(
	ctx context.Context,
	queryer SchemaQueryer,
) (SchemaVerificationSummary, error) {
	if queryer == nil {
		return SchemaVerificationSummary{}, errors.New("schema queryer is required")
	}
	expected, err := AuthoritativeSchemaContracts()
	if err != nil {
		return SchemaVerificationSummary{}, fmt.Errorf("load authoritative schema contract: %w", err)
	}
	expectedTables := sortedContractTableNames(expected)
	actualTables, err := readLiveTableNames(ctx, queryer)
	if err != nil {
		return SchemaVerificationSummary{}, err
	}
	if !reflect.DeepEqual(actualTables, expectedTables) {
		return SchemaVerificationSummary{}, errors.New("database table set differs from the authoritative schema")
	}

	summary := SchemaVerificationSummary{Tables: len(expectedTables)}
	for _, table := range expectedTables {
		actual, err := readSchemaTableContract(ctx, queryer, table)
		if err != nil {
			return SchemaVerificationSummary{}, err
		}
		contract := expected[table]
		summary.Columns += len(contract.Columns)
		summary.Indexes += len(contract.Indexes)
		summary.ForeignKeys += len(contract.ForeignKeys)
		if actual.Engine != contract.Engine || actual.Charset != contract.Charset || actual.Collation != contract.Collation {
			return SchemaVerificationSummary{}, fmt.Errorf("table %s engine or collation differs from the authoritative schema", table)
		}
		if !schemaColumnsEqual(actual.Columns, contract.Columns) {
			return SchemaVerificationSummary{}, fmt.Errorf("table %s columns differ from the authoritative schema: actual=%#v expected=%#v", table, actual.Columns, contract.Columns)
		}
		if !reflect.DeepEqual(actual.Indexes, contract.Indexes) {
			return SchemaVerificationSummary{}, fmt.Errorf("table %s indexes differ from the authoritative schema", table)
		}
		if !reflect.DeepEqual(actual.ForeignKeys, contract.ForeignKeys) {
			return SchemaVerificationSummary{}, fmt.Errorf("table %s foreign keys differ from the authoritative schema", table)
		}
	}
	return summary, nil
}

func readMigrationStatements(path string) ([]string, error) {
	payload, err := fs.ReadFile(migrations.Files, path)
	if err != nil {
		return nil, fmt.Errorf("read migration schema contract %s: %w", path, err)
	}
	statements, err := SplitSQLStatements(string(payload))
	if err != nil {
		return nil, fmt.Errorf("parse migration schema contract %s: %w", path, err)
	}
	return statements, nil
}

func parseCreateTableContracts(statements []string) (map[string]TableContract, error) {
	contracts := make(map[string]TableContract, len(statements))
	for _, statement := range statements {
		matches := schemaCreateTablePattern.FindStringSubmatch(strings.TrimSpace(statement))
		if len(matches) != 6 {
			return nil, fmt.Errorf("cannot parse CREATE TABLE schema contract: %.120s", statement)
		}
		name := matches[1]
		body := matches[2]
		tableCharset := matches[4]
		tableCollation := matches[5]
		columns := make([]ColumnContract, 0)
		for _, definition := range splitSchemaDefinitions(body) {
			if column, ok := parseSchemaColumnContract(definition, tableCharset, tableCollation); ok {
				columns = append(columns, column)
			}
		}
		indexes := make(map[string]IndexContract)
		if primary := schemaPrimaryIndex.FindStringSubmatch(body); len(primary) == 2 {
			indexes["PRIMARY"] = IndexContract{Unique: true, Columns: parseSchemaContractColumns(primary[1])}
		}
		for _, index := range schemaUniqueIndex.FindAllStringSubmatch(body, -1) {
			indexes[index[1]] = IndexContract{Unique: true, Columns: parseSchemaContractColumns(index[2])}
		}
		for _, index := range schemaSecondaryIndex.FindAllStringSubmatch(body, -1) {
			indexes[index[1]] = IndexContract{Columns: parseSchemaContractColumns(index[2])}
		}
		foreignKeys := make(map[string]ForeignKeyContract)
		for _, key := range schemaForeignKey.FindAllStringSubmatch(body, -1) {
			foreignKeys[key[1]] = ForeignKeyContract{
				Columns:           parseSchemaContractColumns(key[2]),
				ReferencedTable:   key[3],
				ReferencedColumns: parseSchemaContractColumns(key[4]),
				UpdateRule:        key[5],
				DeleteRule:        key[6],
			}
		}
		for keyName, key := range foreignKeys {
			if !hasSchemaIndexPrefix(indexes, key.Columns) {
				indexes[keyName] = IndexContract{Columns: append([]string(nil), key.Columns...)}
			}
		}
		if _, duplicate := contracts[name]; duplicate {
			return nil, fmt.Errorf("duplicate CREATE TABLE schema contract for %s", name)
		}
		contracts[name] = TableContract{
			Columns: columns, Indexes: indexes, ForeignKeys: foreignKeys,
			Engine: matches[3], Charset: tableCharset, Collation: tableCollation,
		}
	}
	return contracts, nil
}

func splitSchemaDefinitions(body string) []string {
	definitions := make([]string, 0)
	start, depth := 0, 0
	inString := false
	for index := 0; index < len(body); index++ {
		switch body[index] {
		case '\'':
			if inString && index+1 < len(body) && body[index+1] == '\'' {
				index++
				continue
			}
			inString = !inString
		case '(':
			if !inString {
				depth++
			}
		case ')':
			if !inString && depth > 0 {
				depth--
			}
		case ',':
			if !inString && depth == 0 {
				definitions = append(definitions, strings.TrimSpace(body[start:index]))
				start = index + 1
			}
		}
	}
	if tail := strings.TrimSpace(body[start:]); tail != "" {
		definitions = append(definitions, tail)
	}
	return definitions
}

func schemaColumnsEqual(actual, expected []ColumnContract) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range actual {
		left, right := actual[index], expected[index]
		left.GenerationExpression = normalizeSchemaGenerationExpression(left.GenerationExpression)
		right.GenerationExpression = normalizeSchemaGenerationExpression(right.GenerationExpression)
		if !reflect.DeepEqual(left, right) {
			return false
		}
	}
	return true
}

func normalizeSchemaGenerationExpression(value string) string {
	value = strings.ToLower(strings.ReplaceAll(value, "`", ""))
	value = strings.Join(strings.Fields(value), " ")
	for len(value) >= 2 && value[0] == '(' && value[len(value)-1] == ')' {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	if start := strings.Index(value, "case when ("); start >= 0 {
		conditionStart := start + len("case when ")
		if conditionEnd := strings.Index(value[conditionStart:], ") then"); conditionEnd >= 0 {
			conditionEnd += conditionStart
			value = value[:conditionStart] + value[conditionStart+1:conditionEnd] + value[conditionEnd+1:]
		}
	}
	return value
}

func parseSchemaColumnContract(line, tableCharset, tableCollation string) (ColumnContract, bool) {
	definition := strings.TrimSuffix(strings.TrimSpace(line), ",")
	fields := strings.Fields(definition)
	if len(fields) < 2 {
		return ColumnContract{}, false
	}
	name := strings.Trim(fields[0], "`")
	if !schemaColumnNamePattern.MatchString(name) {
		return ColumnContract{}, false
	}
	columnType := strings.ToLower(fields[1])
	if !schemaColumnTypePattern.MatchString(columnType) {
		return ColumnContract{}, false
	}
	contract := ColumnContract{Name: name, ColumnType: columnType, IsNullable: "YES"}
	upperDefinition := strings.ToUpper(definition)
	if strings.Contains(upperDefinition, " NOT NULL") {
		contract.IsNullable = "NO"
	}
	if strings.Contains(upperDefinition, " AUTO_INCREMENT") {
		contract.Extra = "auto_increment"
	}
	if matched := schemaColumnGenerated.FindStringSubmatch(definition); len(matched) == 2 {
		contract.GenerationExpression = strings.TrimSpace(matched[1])
		if strings.Contains(upperDefinition, " STORED") {
			contract.Extra = "STORED GENERATED"
		} else {
			contract.Extra = "VIRTUAL GENERATED"
		}
	}
	if matched := schemaColumnDefault.FindStringSubmatch(definition); len(matched) == 2 {
		value := matched[1]
		if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
			value = strings.ReplaceAll(value[1:len(value)-1], "''", "'")
		}
		if !strings.EqualFold(value, "NULL") {
			contract.Default = sql.NullString{String: value, Valid: true}
		}
	}
	if isSchemaCharacterColumnType(columnType) {
		charset := tableCharset
		if matched := schemaColumnCharset.FindStringSubmatch(definition); len(matched) == 2 {
			charset = matched[1]
		}
		collation := tableCollation
		if matched := schemaColumnCollation.FindStringSubmatch(definition); len(matched) == 2 {
			collation = matched[1]
		}
		contract.CharacterSet = sql.NullString{String: charset, Valid: true}
		contract.Collation = sql.NullString{String: collation, Valid: true}
	}
	return contract, true
}

func isSchemaCharacterColumnType(columnType string) bool {
	baseType := columnType
	if index := strings.IndexByte(baseType, '('); index >= 0 {
		baseType = baseType[:index]
	}
	switch baseType {
	case "char", "varchar", "tinytext", "text", "mediumtext", "longtext":
		return true
	default:
		return false
	}
}

func hasSchemaIndexPrefix(indexes map[string]IndexContract, columns []string) bool {
	for _, index := range indexes {
		if len(index.Columns) < len(columns) {
			continue
		}
		matches := true
		for position, column := range columns {
			if index.Columns[position] != column {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func parseSchemaContractColumns(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, strings.Trim(strings.TrimSpace(part), "`"))
	}
	return result
}

func applySchemaCollectionRunScopeContract(contracts map[string]TableContract) {
	contract := contracts["collection_run"]
	columns := make([]ColumnContract, 0, len(contract.Columns)+1)
	for _, column := range contract.Columns {
		columns = append(columns, column)
		if column.Name == "end_timestamp" {
			columns = append(columns, ColumnContract{Name: "scope", ColumnType: "json", IsNullable: "NO"})
		}
	}
	contract.Columns = columns
	contracts["collection_run"] = contract
}

func applySchemaAlertReliabilityContract(contracts map[string]TableContract) {
	delivery := contracts["alert_delivery"]
	columns := make([]ColumnContract, 0, len(delivery.Columns)+3)
	for _, column := range delivery.Columns {
		columns = append(columns, column)
		if column.Name == "attempt_count" {
			columns = append(columns,
				ColumnContract{
					Name: "claim_token", ColumnType: "varchar(64)", IsNullable: "YES",
					CharacterSet: sql.NullString{String: "ascii", Valid: true},
					Collation:    sql.NullString{String: "ascii_bin", Valid: true},
				},
				ColumnContract{Name: "lease_expires_at", ColumnType: "bigint", IsNullable: "YES"},
				ColumnContract{Name: "payload_snapshot", ColumnType: "json", IsNullable: "NO"},
			)
		}
	}
	delivery.Columns = columns
	delivery.Indexes["idx_alert_delivery_claim"] = IndexContract{
		Columns: []string{"status", "lease_expires_at", "next_retry_at", "id"},
	}
	contracts["alert_delivery"] = delivery
	contracts["alert_evaluation_cursor"] = TableContract{
		Columns: []ColumnContract{
			{
				Name: "active_key", ColumnType: "varchar(384)", IsNullable: "NO",
				CharacterSet: sql.NullString{String: "utf8mb4", Valid: true},
				Collation:    sql.NullString{String: "utf8mb4_bin", Valid: true},
			},
			{Name: "last_sample_at", ColumnType: "bigint", IsNullable: "NO"},
			{
				Name: "last_sample_key", ColumnType: "varchar(255)", IsNullable: "NO",
				CharacterSet: sql.NullString{String: "utf8mb4", Valid: true},
				Collation:    sql.NullString{String: "utf8mb4_bin", Valid: true},
			},
			{Name: "created_at", ColumnType: "bigint", IsNullable: "NO"},
			{Name: "updated_at", ColumnType: "bigint", IsNullable: "NO"},
		},
		Indexes:     map[string]IndexContract{"PRIMARY": {Unique: true, Columns: []string{"active_key"}}},
		ForeignKeys: map[string]ForeignKeyContract{},
		Engine:      "InnoDB", Charset: "utf8mb4", Collation: "utf8mb4_unicode_ci",
	}
}

func applySchemaExportClaimLeaseContract(contracts map[string]TableContract) {
	exportJob := contracts["export_job"]
	columns := make([]ColumnContract, 0, len(exportJob.Columns)+2)
	for _, column := range exportJob.Columns {
		columns = append(columns, column)
		if column.Name == "heartbeat_at" {
			columns = append(columns,
				ColumnContract{
					Name: "claim_token", ColumnType: "varchar(64)", IsNullable: "YES",
					CharacterSet: sql.NullString{String: "ascii", Valid: true},
					Collation:    sql.NullString{String: "ascii_bin", Valid: true},
				},
				ColumnContract{Name: "lease_expires_at", ColumnType: "bigint", IsNullable: "YES"},
			)
		}
	}
	exportJob.Columns = columns
	exportJob.Indexes["idx_export_job_claim"] = IndexContract{
		Columns: []string{"status", "lease_expires_at", "next_attempt_at", "id"},
	}
	contracts["export_job"] = exportJob
}

func applySchemaUsageFactCapacityIndexContract(contracts map[string]TableContract) {
	contract := contracts["usage_fact_hourly"]
	contract.Indexes["idx_usage_fact_hourly_time_user"] = IndexContract{
		Columns: []string{"hour_ts", "site_id", "remote_user_id"},
	}
	contract.Indexes["idx_usage_fact_hourly_time_model_user"] = IndexContract{
		Columns: []string{"hour_ts", "site_id", "model_name", "remote_user_id"},
	}
	contract.Indexes["idx_usage_fact_hourly_time_channel_user"] = IndexContract{
		Columns: []string{"hour_ts", "site_id", "channel_id", "remote_user_id"},
	}
	contracts["usage_fact_hourly"] = contract
}

func applySchemaAlertLifecycleContract(contracts map[string]TableContract) {
	alertEvent := contracts["alert_event"]
	alertColumns := make([]ColumnContract, 0, len(alertEvent.Columns)+1)
	for _, column := range alertEvent.Columns {
		alertColumns = append(alertColumns, column)
		if column.Name == "resolved_at" {
			alertColumns = append(alertColumns, ColumnContract{
				Name: "resolution_reason", ColumnType: "varchar(32)", IsNullable: "YES",
				CharacterSet: sql.NullString{String: "utf8mb4", Valid: true},
				Collation:    sql.NullString{String: "utf8mb4_unicode_ci", Valid: true},
			})
		}
	}
	alertEvent.Columns = alertColumns
	contracts["alert_event"] = alertEvent

	instances := contracts["site_instance"]
	instanceColumns := make([]ColumnContract, 0, len(instances.Columns)+1)
	for _, column := range instances.Columns {
		instanceColumns = append(instanceColumns, column)
		if column.Name == "updated_at" {
			instanceColumns = append(instanceColumns, ColumnContract{Name: "retired_at", ColumnType: "bigint", IsNullable: "YES"})
		}
	}
	instances.Columns = instanceColumns
	instances.Indexes["idx_site_instance_active"] = IndexContract{Columns: []string{"site_id", "retired_at", "node_name"}}
	contracts["site_instance"] = instances
}

func migrationProgressTableContract() TableContract {
	return TableContract{
		Columns: []ColumnContract{
			{Name: "version", ColumnType: "varchar(64)", IsNullable: "NO", CharacterSet: sql.NullString{String: "utf8mb4", Valid: true}, Collation: sql.NullString{String: "utf8mb4_unicode_ci", Valid: true}},
			{Name: "checksum", ColumnType: "char(64)", IsNullable: "NO", CharacterSet: sql.NullString{String: "utf8mb4", Valid: true}, Collation: sql.NullString{String: "utf8mb4_unicode_ci", Valid: true}},
			{Name: "statement_index", ColumnType: "int", IsNullable: "NO", Default: sql.NullString{String: "0", Valid: true}},
			{Name: "state", ColumnType: "varchar(16)", IsNullable: "NO", Default: sql.NullString{String: "ready", Valid: true}, CharacterSet: sql.NullString{String: "utf8mb4", Valid: true}, Collation: sql.NullString{String: "utf8mb4_unicode_ci", Valid: true}},
			{Name: "updated_at", ColumnType: "bigint", IsNullable: "NO"},
		},
		Indexes:     map[string]IndexContract{"PRIMARY": {Unique: true, Columns: []string{"version"}}},
		ForeignKeys: map[string]ForeignKeyContract{},
		Engine:      "InnoDB", Charset: "utf8mb4", Collation: "utf8mb4_unicode_ci",
	}
}

func sortedContractTableNames(contracts map[string]TableContract) []string {
	names := make([]string, 0, len(contracts))
	for name := range contracts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func readLiveTableNames(ctx context.Context, queryer SchemaQueryer) ([]string, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT table_name
FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'
ORDER BY table_name`)
	if err != nil {
		return nil, fmt.Errorf("list live schema tables: %w", err)
	}
	defer func() { _ = rows.Close() }()
	names := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan live schema table: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate live schema tables: %w", err)
	}
	return names, nil
}

func readSchemaTableContract(ctx context.Context, queryer SchemaQueryer, table string) (TableContract, error) {
	var engine, charset, collation string
	if err := queryer.QueryRowContext(ctx, `SELECT t.engine, c.character_set_name, t.table_collation
FROM information_schema.tables t
JOIN information_schema.collation_character_set_applicability c
  ON c.collation_name = t.table_collation
WHERE t.table_schema = DATABASE() AND t.table_name = ?`, table).Scan(&engine, &charset, &collation); err != nil {
		return TableContract{}, fmt.Errorf("read table %s engine and collation: %w", table, err)
	}

	columnRows, err := queryer.QueryContext(ctx, `SELECT column_name, column_type, is_nullable, column_default,
       extra, generation_expression, character_set_name, collation_name
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ?
ORDER BY ordinal_position`, table)
	if err != nil {
		return TableContract{}, fmt.Errorf("read table %s columns: %w", table, err)
	}
	columns := make([]ColumnContract, 0)
	for columnRows.Next() {
		var column ColumnContract
		if err := columnRows.Scan(
			&column.Name, &column.ColumnType, &column.IsNullable, &column.Default,
			&column.Extra, &column.GenerationExpression, &column.CharacterSet, &column.Collation,
		); err != nil {
			_ = columnRows.Close()
			return TableContract{}, fmt.Errorf("scan table %s column: %w", table, err)
		}
		columns = append(columns, column)
	}
	if err := columnRows.Err(); err != nil {
		_ = columnRows.Close()
		return TableContract{}, fmt.Errorf("iterate table %s columns: %w", table, err)
	}
	if err := columnRows.Close(); err != nil {
		return TableContract{}, fmt.Errorf("close table %s columns: %w", table, err)
	}

	indexes := make(map[string]IndexContract)
	indexRows, err := queryer.QueryContext(ctx, `SELECT index_name, non_unique, COALESCE(column_name, '')
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ?
ORDER BY index_name, seq_in_index`, table)
	if err != nil {
		return TableContract{}, fmt.Errorf("read table %s indexes: %w", table, err)
	}
	for indexRows.Next() {
		var name, column string
		var nonUnique int
		if err := indexRows.Scan(&name, &nonUnique, &column); err != nil {
			_ = indexRows.Close()
			return TableContract{}, fmt.Errorf("scan table %s index: %w", table, err)
		}
		if column == "" {
			_ = indexRows.Close()
			return TableContract{}, fmt.Errorf("table %s contains an unsupported functional index", table)
		}
		contract := indexes[name]
		contract.Unique = nonUnique == 0
		contract.Columns = append(contract.Columns, column)
		indexes[name] = contract
	}
	if err := indexRows.Err(); err != nil {
		_ = indexRows.Close()
		return TableContract{}, fmt.Errorf("iterate table %s indexes: %w", table, err)
	}
	if err := indexRows.Close(); err != nil {
		return TableContract{}, fmt.Errorf("close table %s indexes: %w", table, err)
	}

	foreignKeys := make(map[string]ForeignKeyContract)
	foreignKeyRows, err := queryer.QueryContext(ctx, `SELECT k.constraint_name, k.column_name, k.referenced_table_name,
       k.referenced_column_name, r.update_rule, r.delete_rule
FROM information_schema.key_column_usage k
JOIN information_schema.referential_constraints r
  ON r.constraint_schema = k.constraint_schema
 AND r.table_name = k.table_name
 AND r.constraint_name = k.constraint_name
WHERE k.table_schema = DATABASE()
  AND k.table_name = ?
  AND k.referenced_table_name IS NOT NULL
ORDER BY k.constraint_name, k.ordinal_position`, table)
	if err != nil {
		return TableContract{}, fmt.Errorf("read table %s foreign keys: %w", table, err)
	}
	for foreignKeyRows.Next() {
		var name, column, referencedTable, referencedColumn, updateRule, deleteRule string
		if err := foreignKeyRows.Scan(&name, &column, &referencedTable, &referencedColumn, &updateRule, &deleteRule); err != nil {
			_ = foreignKeyRows.Close()
			return TableContract{}, fmt.Errorf("scan table %s foreign key: %w", table, err)
		}
		contract := foreignKeys[name]
		contract.Columns = append(contract.Columns, column)
		contract.ReferencedTable = referencedTable
		contract.ReferencedColumns = append(contract.ReferencedColumns, referencedColumn)
		contract.UpdateRule = updateRule
		contract.DeleteRule = deleteRule
		foreignKeys[name] = contract
	}
	if err := foreignKeyRows.Err(); err != nil {
		_ = foreignKeyRows.Close()
		return TableContract{}, fmt.Errorf("iterate table %s foreign keys: %w", table, err)
	}
	if err := foreignKeyRows.Close(); err != nil {
		return TableContract{}, fmt.Errorf("close table %s foreign keys: %w", table, err)
	}
	return TableContract{
		Columns: columns, Indexes: indexes, ForeignKeys: foreignKeys,
		Engine: engine, Charset: charset, Collation: collation,
	}, nil
}
