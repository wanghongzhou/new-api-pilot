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
	statements, err := readMigrationStatements("0001_initial_schema.sql")
	if err != nil {
		return nil, err
	}
	return parseCreateTableContracts(statements)
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
