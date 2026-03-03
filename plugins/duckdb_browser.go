package plugins

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/qbart/ferrodb/ferro/config"
	"github.com/qbart/ferrodb/ferro/plugin"
)

type DuckDBBrowser struct {
	driver *DuckDBDriver
	conn   *DuckDBDriverConnection
}

func NewDuckDBBrowser(driver *DuckDBDriver) *DuckDBBrowser {
	return &DuckDBBrowser{driver: driver}
}

func (b *DuckDBBrowser) Connect(ctx context.Context, dsn string) error {
	conn, err := b.driver.Connect(ctx, config.DriverConfig{"path": dsn})
	if err != nil {
		return err
	}
	b.conn = conn.(*DuckDBDriverConnection)
	return nil
}

func (b *DuckDBBrowser) Disconnect(ctx context.Context) error {
	return b.driver.Disconnect(ctx, b.conn)
}

// List navigates a hierarchy:
//
//	[]                          → schemas
//	[schema]                    → object type categories (Tables, Views, Sequences, Macros)
//	[schema, type]              → objects of that type
//	[schema, "table", name]     → sub-categories (Columns, Indexes, Constraints)
//	[schema, "table", name, cat]→ items within that category
func (b *DuckDBBrowser) List(ctx context.Context, ids []string) ([]plugin.BrowserItem, error) {
	switch len(ids) {
	case 0:
		return b.listSchemas(ctx)

	case 1:
		return []plugin.BrowserItem{
			{ID: "table", Name: "Tables", HasChildren: true},
			{ID: "view", Name: "Views", HasChildren: true},
			{ID: "sequence", Name: "Sequences", HasChildren: true},
			{ID: "macro", Name: "Macros", HasChildren: true},
		}, nil

	case 2:
		schema, objectType := ids[0], ids[1]
		switch objectType {
		case "table":
			return b.listTables(ctx, schema)
		case "view":
			return b.listViews(ctx, schema)
		case "sequence":
			return b.listSequences(ctx, schema)
		case "macro":
			return b.listMacros(ctx, schema)
		}

	case 3:
		if ids[1] == "table" {
			return []plugin.BrowserItem{
				{ID: "column", Name: "Columns", HasChildren: true},
				{ID: "index", Name: "Indexes", HasChildren: true},
				{ID: "constraint", Name: "Constraints", HasChildren: true},
			}, nil
		}

	case 4:
		schema, objectType, tableName, category := ids[0], ids[1], ids[2], ids[3]
		if objectType == "table" {
			switch category {
			case "column":
				return b.listColumns(ctx, schema, tableName)
			case "index":
				return b.listIndexes(ctx, schema, tableName)
			case "constraint":
				return b.listConstraints(ctx, schema, tableName)
			}
		}
	}
	return nil, fmt.Errorf("unsupported path depth: %d", len(ids))
}

// Show returns a query that previews the selected object.
func (b *DuckDBBrowser) Show(ctx context.Context, ids []string) (string, error) {
	if len(ids) < 3 {
		return "", nil
	}
	schema, objectType, name := ids[0], ids[1], ids[2]

	switch objectType {
	case "table", "view":
		return fmt.Sprintf("SELECT * FROM %s.%s LIMIT 100",
			duckdbQuoteIdent(schema), duckdbQuoteIdent(name)), nil
	case "sequence":
		return fmt.Sprintf(
			`SELECT * FROM duckdb_sequences() WHERE schema_name = '%s' AND sequence_name = '%s'`,
			strings.ReplaceAll(schema, "'", "''"),
			strings.ReplaceAll(name, "'", "''"),
		), nil
	case "macro":
		return fmt.Sprintf(
			`SELECT * FROM duckdb_functions() WHERE schema_name = '%s' AND function_name = '%s' AND function_type = 'macro'`,
			strings.ReplaceAll(schema, "'", "''"),
			strings.ReplaceAll(name, "'", "''"),
		), nil
	}
	return "", nil
}

// Query executes arbitrary SQL against the database.
func (b *DuckDBBrowser) Query(ctx context.Context, query string) (plugin.BrowserQueryResult, error) {
	rows, err := b.conn.DB.QueryContext(ctx, query)
	if err != nil {
		return plugin.BrowserQueryResult{}, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return plugin.BrowserQueryResult{}, err
	}

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return plugin.BrowserQueryResult{}, err
	}

	editorTypes := make([]string, len(colTypes))
	for i, ct := range colTypes {
		editorTypes[i] = duckdbTypeToEditorType(ct.DatabaseTypeName())
	}

	var data [][]string
	for rows.Next() {
		values := make([]any, len(cols))
		valuePtrs := make([]any, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return plugin.BrowserQueryResult{}, err
		}
		row := make([]string, len(values))
		for i, v := range values {
			row[i] = duckdbBrowserValueToString(v)
		}
		data = append(data, row)
	}
	if err := rows.Err(); err != nil {
		return plugin.BrowserQueryResult{}, err
	}

	return plugin.BrowserQueryResult{Headers: cols, Rows: data, ColumnTypes: editorTypes}, nil
}

// ParseExplain parses DuckDB EXPLAIN ANALYZE output.
//
// DuckDB EXPLAIN produces a text-based plan. The EXPLAIN output is returned
// as a single-column result with rows of text describing the physical plan.
func (b *DuckDBBrowser) ParseExplain(data plugin.BrowserQueryResult) (plugin.BrowserExplainResult, error) {
	if len(data.Rows) == 0 {
		return plugin.BrowserExplainResult{}, fmt.Errorf(
			"query result does not match DuckDB EXPLAIN output (no rows)",
		)
	}

	// DuckDB EXPLAIN returns rows with explain_key and explain_value columns,
	// or a single-column text output. Collect all text lines.
	var lines []string
	for _, row := range data.Rows {
		for _, cell := range row {
			if cell != "" {
				lines = append(lines, cell)
			}
		}
	}

	var explainRows []plugin.BrowserExplainRow
	for _, line := range lines {
		explainRows = append(explainRows, plugin.BrowserExplainRow{
			Cells:     []string{line},
			Highlight: duckdbIsKeyPlanLine(line),
		})
	}

	root := duckdbBuildLogicalPlan(lines)

	summary := []plugin.BrowserExplainLine{
		{Text: fmt.Sprintf("Plan lines: %d", len(lines))},
	}

	planTable := plugin.BrowserExplainTable{
		Title:   "Physical Plan",
		Headers: []string{"plan"},
		Rows:    explainRows,
	}

	return plugin.BrowserExplainResult{
		Root:         root,
		SummaryLines: summary,
		Tables:       []plugin.BrowserExplainTable{planTable},
	}, nil
}

// duckdbBuildLogicalPlan parses the indented text plan from DuckDB EXPLAIN
// into a tree of BrowserExplainNodes.
func duckdbBuildLogicalPlan(lines []string) plugin.BrowserExplainNode {
	root := plugin.BrowserExplainNode{Name: "Query Plan"}

	type stackEntry struct {
		node  *plugin.BrowserExplainNode
		depth int
	}

	stack := []stackEntry{{node: &root, depth: -1}}

	for _, line := range lines {
		trimmed := strings.TrimLeft(line, "│ ├─└─")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}

		// Calculate indentation depth
		depth := len(line) - len(strings.TrimLeft(line, " │├└─"))

		node := plugin.BrowserExplainNode{
			Name: trimmed,
		}

		// Find parent: walk back stack to find entry with smaller depth
		for len(stack) > 1 && stack[len(stack)-1].depth >= depth {
			stack = stack[:len(stack)-1]
		}

		parent := stack[len(stack)-1].node
		parent.Children = append(parent.Children, node)
		// Get pointer to the just-appended child
		childPtr := &parent.Children[len(parent.Children)-1]
		stack = append(stack, stackEntry{node: childPtr, depth: depth})
	}

	return root
}

// duckdbIsKeyPlanLine returns true for plan lines that are significant.
func duckdbIsKeyPlanLine(line string) bool {
	upper := strings.ToUpper(line)
	keywords := []string{
		"SEQ_SCAN", "INDEX_SCAN", "HASH_JOIN", "NESTED_LOOP",
		"MERGE_JOIN", "FILTER", "ORDER_BY", "AGGREGATE",
		"HASH_GROUP_BY", "PERFECT_HASH_GROUP_BY",
		"TABLE_SCAN", "PARQUET_SCAN", "CSV_SCAN",
	}
	for _, kw := range keywords {
		if strings.Contains(upper, kw) {
			return true
		}
	}
	return false
}

// --- list helpers ---

func (b *DuckDBBrowser) listSchemas(ctx context.Context) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT schema_name FROM information_schema.schemata
		 WHERE catalog_name = current_database()
		   AND schema_name NOT IN ('information_schema', 'pg_catalog')
		 ORDER BY schema_name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return duckdbCollectItems(rows, true)
}

func (b *DuckDBBrowser) listTables(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = $1
		   AND table_type = 'BASE TABLE'
		   AND table_name NOT LIKE '_ferro_%'
		 ORDER BY table_name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return duckdbCollectItems(rows, true)
}

func (b *DuckDBBrowser) listViews(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = $1 AND table_type = 'VIEW'
		 ORDER BY table_name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return duckdbCollectItems(rows, false)
}

func (b *DuckDBBrowser) listSequences(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT sequence_name FROM duckdb_sequences()
		 WHERE schema_name = $1
		 ORDER BY sequence_name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return duckdbCollectItems(rows, false)
}

func (b *DuckDBBrowser) listMacros(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT DISTINCT function_name FROM duckdb_functions()
		 WHERE schema_name = $1 AND function_type = 'macro'
		 ORDER BY function_name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return duckdbCollectItems(rows, false)
}

func (b *DuckDBBrowser) listColumns(ctx context.Context, schema, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT column_name, data_type, is_nullable, column_default
		 FROM information_schema.columns
		 WHERE table_schema = $1 AND table_name = $2
		 ORDER BY ordinal_position`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []plugin.BrowserItem
	for rows.Next() {
		var name, dataType, nullable string
		var dflt sql.NullString
		if err := rows.Scan(&name, &dataType, &nullable, &dflt); err != nil {
			return nil, err
		}
		label := name + " (" + dataType + ")"
		if nullable == "NO" {
			label += " NOT NULL"
		}
		items = append(items, plugin.BrowserItem{ID: name, Name: label, HasChildren: false})
	}
	return items, rows.Err()
}

func (b *DuckDBBrowser) listIndexes(ctx context.Context, schema, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT index_name, is_unique
		 FROM duckdb_indexes()
		 WHERE schema_name = $1 AND table_name = $2
		 ORDER BY index_name`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []plugin.BrowserItem
	for rows.Next() {
		var name string
		var isUnique bool
		if err := rows.Scan(&name, &isUnique); err != nil {
			return nil, err
		}
		label := name
		if isUnique {
			label += " (unique)"
		}
		items = append(items, plugin.BrowserItem{ID: name, Name: label, HasChildren: false})
	}
	return items, rows.Err()
}

func (b *DuckDBBrowser) listConstraints(ctx context.Context, schema, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT constraint_text, constraint_type
		 FROM duckdb_constraints()
		 WHERE schema_name = $1 AND table_name = $2
		 ORDER BY constraint_type`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []plugin.BrowserItem
	for rows.Next() {
		var text string
		var cType string
		if err := rows.Scan(&text, &cType); err != nil {
			return nil, err
		}
		label := cType
		if text != "" {
			label += ": " + text
		}
		items = append(items, plugin.BrowserItem{ID: text, Name: label, HasChildren: false})
	}
	return items, rows.Err()
}

// --- shared helpers ---

func duckdbCollectItems(rows *sql.Rows, hasChildren bool) ([]plugin.BrowserItem, error) {
	var items []plugin.BrowserItem
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		items = append(items, plugin.BrowserItem{ID: name, Name: name, HasChildren: hasChildren})
	}
	return items, rows.Err()
}

func duckdbTypeToEditorType(dbTypeName string) string {
	upper := strings.ToUpper(dbTypeName)
	switch {
	case upper == "JSON":
		return plugin.EditorTypeObject
	case strings.Contains(upper, "INT"),
		strings.Contains(upper, "BIGINT"),
		strings.Contains(upper, "SMALLINT"),
		strings.Contains(upper, "TINYINT"),
		strings.Contains(upper, "HUGEINT"):
		return plugin.EditorTypeInt64
	case strings.Contains(upper, "FLOAT"),
		strings.Contains(upper, "DOUBLE"),
		strings.Contains(upper, "DECIMAL"),
		strings.Contains(upper, "NUMERIC"),
		strings.Contains(upper, "REAL"):
		return plugin.EditorTypeFloat64
	default:
		return plugin.EditorTypeString
	}
}

func duckdbBrowserValueToString(v any) string {
	if v == nil {
		return "NULL"
	}
	if b, ok := v.([]byte); ok {
		if utf8.Valid(b) {
			return string(b)
		}
		return "0x" + hex.EncodeToString(b)
	}
	// DuckDB may return maps/structs as JSON-like values
	if m, ok := v.(map[string]any); ok {
		b, err := json.Marshal(m)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
	return fmt.Sprintf("%v", v)
}
