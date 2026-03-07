package plugins

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qbart/ferrodb/ferro/config"
	"github.com/qbart/ferrodb/ferro/plugin"
)

type ClickHouseBrowser struct {
	driver *ClickHouseDriver
	conn   *ClickHouseDriverConnection
}

func NewClickHouseBrowser(driver *ClickHouseDriver) *ClickHouseBrowser {
	return &ClickHouseBrowser{driver: driver}
}

func (b *ClickHouseBrowser) Connect(ctx context.Context, dsn string) error {
	conn, err := b.driver.Connect(ctx, config.DriverConfig{"dsn": dsn})
	if err != nil {
		return err
	}
	b.conn = conn.(*ClickHouseDriverConnection)
	return nil
}

func (b *ClickHouseBrowser) Disconnect(ctx context.Context) error {
	return b.driver.Disconnect(ctx, b.conn)
}

func (b *ClickHouseBrowser) Show(ctx context.Context, ids []string) (string, error) {
	if len(ids) < 3 {
		return "", nil
	}
	database, objectType, name := ids[0], ids[1], ids[2]

	// table sub-item: [database, "table", table_name, category, item_name]
	if (objectType == "table" || objectType == "view" || objectType == "matview") && len(ids) == 5 {
		category, itemName := ids[3], ids[4]
		switch category {
		case "index":
			return fmt.Sprintf(
				`SELECT
    name,
    type,
    expr,
    granularity,
    data_compressed_bytes,
    data_uncompressed_bytes
FROM system.data_skipping_indices
WHERE database = '%s' AND table = '%s' AND name = '%s'`,
				database, ids[2], itemName,
			), nil
		}
		return "", nil
	}

	switch objectType {
	case "table", "view", "matview":
		return fmt.Sprintf("SELECT * FROM %s.%s LIMIT 100",
			chQuoteIdent(database), chQuoteIdent(name)), nil
	case "dictionary":
		return fmt.Sprintf("SELECT * FROM %s.%s LIMIT 100",
			chQuoteIdent(database), chQuoteIdent(name)), nil
	case "function":
		return fmt.Sprintf(
			`SELECT name, create_query FROM system.functions WHERE name = '%s' AND origin = 'SQLUserDefined'`,
			name,
		), nil
	}
	return "", nil
}

func chTypeToEditorType(dbTypeName string) string {
	upper := strings.ToUpper(dbTypeName)
	switch {
	case upper == "JSON",
		strings.HasPrefix(upper, "MAP("),
		strings.HasPrefix(upper, "TUPLE("),
		strings.HasPrefix(upper, "NESTED("):
		return plugin.EditorTypeObject
	case strings.Contains(upper, "INT"),
		upper == "ENUM8",
		upper == "ENUM16":
		return plugin.EditorTypeInt64
	case strings.Contains(upper, "FLOAT"),
		strings.Contains(upper, "DOUBLE"),
		strings.Contains(upper, "DECIMAL"):
		return plugin.EditorTypeFloat64
	default:
		return plugin.EditorTypeString
	}
}

func chValueToString(v any) string {
	if v == nil {
		return "NULL"
	}
	if b, ok := v.([]byte); ok {
		if utf8.Valid(b) {
			return string(b)
		}
		return "0x" + hex.EncodeToString(b)
	}
	if t, ok := v.(time.Time); ok {
		return t.Format("2006-01-02 15:04:05.999999")
	}
	return fmt.Sprintf("%v", v)
}

func (b *ClickHouseBrowser) Query(ctx context.Context, query string) (plugin.BrowserQueryResult, error) {
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
		editorTypes[i] = chTypeToEditorType(ct.DatabaseTypeName())
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
			row[i] = chValueToString(v)
		}
		data = append(data, row)
	}
	if err := rows.Err(); err != nil {
		return plugin.BrowserQueryResult{}, err
	}

	return plugin.BrowserQueryResult{Headers: cols, Rows: data, ColumnTypes: editorTypes}, nil
}

// chExplain types model the ClickHouse EXPLAIN json = 1 output.

type chExplainResult struct {
	Plan chExplainPlan `json:"Plan"`
}

type chExplainPlan struct {
	NodeType        string          `json:"Node Type"`
	NodeID          string          `json:"Node Id,omitempty"`
	Description     string          `json:"Description,omitempty"`
	Header          []string        `json:"Header,omitempty"`
	ReadType        string          `json:"Read Type,omitempty"`
	Database        string          `json:"Database,omitempty"`
	Table           string          `json:"Table,omitempty"`
	ReadingFromPart string          `json:"Reading from Part,omitempty"`
	ReadRows        any             `json:"Read Rows,omitempty"`
	ReadBytes       any             `json:"Read Bytes,omitempty"`
	Expression      string          `json:"Expression,omitempty"`
	Actions         string          `json:"Actions,omitempty"`
	Positions       []int           `json:"Positions,omitempty"`
	FilterColumn    string          `json:"Filter Column,omitempty"`
	Parts           any             `json:"Parts,omitempty"`
	Granules        any             `json:"Granules,omitempty"`
	Indexes         []chExplainIdx  `json:"Indexes,omitempty"`
	SortingKey      string          `json:"Sorting Key,omitempty"`
	Plans           []chExplainPlan `json:"Plans,omitempty"`
}

type chExplainIdx struct {
	Type        string `json:"Type"`
	Name        string `json:"Name,omitempty"`
	Description string `json:"Description,omitempty"`
	Keys        []string `json:"Keys,omitempty"`
	Condition   string `json:"Condition,omitempty"`
	InitialParts any    `json:"Initial Parts,omitempty"`
	SelectedParts any   `json:"Selected Parts,omitempty"`
	InitialGranules any `json:"Initial Granules,omitempty"`
	SelectedGranules any `json:"Selected Granules,omitempty"`
}

func (b *ClickHouseBrowser) ParseExplain(data plugin.BrowserQueryResult) (plugin.BrowserExplainResult, error) {
	if len(data.Rows) == 0 || len(data.Headers) == 0 {
		return plugin.BrowserExplainResult{}, fmt.Errorf("query result does not match the explain output")
	}

	// Collect all row text
	var sb strings.Builder
	for _, row := range data.Rows {
		if len(row) > 0 {
			sb.WriteString(row[0])
			sb.WriteString("\n")
		}
	}
	raw := strings.TrimSpace(sb.String())

	// Try [{"Plan": {...}}] format (EXPLAIN json = 1)
	var results []chExplainResult
	if err := json.Unmarshal([]byte(raw), &results); err == nil && len(results) > 0 && results[0].Plan.NodeType != "" {
		return chParseExplainJSON(results[0].Plan), nil
	}

	// Try bare plan object
	var plan chExplainPlan
	if err := json.Unmarshal([]byte(raw), &plan); err == nil && plan.NodeType != "" {
		return chParseExplainJSON(plan), nil
	}

	// Fall back to text-based EXPLAIN
	return chParseExplainText(raw), nil
}

func chParseExplainJSON(plan chExplainPlan) plugin.BrowserExplainResult {
	root := chPlanToNode(plan)

	var tables []plugin.BrowserExplainTable
	var leaves []chExplainPlan
	chCollectLeaves(plan, &leaves)

	if len(leaves) > 0 {
		table := plugin.BrowserExplainTable{
			Title:   "Read operations",
			Headers: []string{"table", "read type", "parts", "granules"},
		}
		for _, l := range leaves {
			if l.Table == "" {
				continue
			}
			tableName := l.Table
			if l.Database != "" {
				tableName = l.Database + "." + l.Table
			}
			table.Rows = append(table.Rows, plugin.BrowserExplainRow{
				Cells: []string{
					tableName,
					l.ReadType,
					fmt.Sprintf("%v", l.Parts),
					fmt.Sprintf("%v", l.Granules),
				},
			})
		}
		if len(table.Rows) > 0 {
			tables = append(tables, table)
		}
	}

	return plugin.BrowserExplainResult{
		Root:   root,
		Tables: tables,
	}
}

func chCollectLeaves(plan chExplainPlan, out *[]chExplainPlan) {
	if plan.Table != "" {
		*out = append(*out, plan)
	}
	for _, child := range plan.Plans {
		chCollectLeaves(child, out)
	}
}

func chPlanToNode(plan chExplainPlan) plugin.BrowserExplainNode {
	name := plan.NodeType
	if plan.Table != "" {
		tableName := plan.Table
		if plan.Database != "" {
			tableName = plan.Database + "." + plan.Table
		}
		name += " on " + tableName
	}

	node := plugin.BrowserExplainNode{Name: name}

	if plan.Description != "" {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{Text: plan.Description})
	}

	if plan.ReadType != "" {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{Text: "read type: " + plan.ReadType})
	}

	if plan.Expression != "" {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{Text: "expression: " + plan.Expression})
	}

	if plan.Actions != "" {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{Text: "actions: " + plan.Actions})
	}

	if plan.FilterColumn != "" {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{Text: "filter: " + plan.FilterColumn, Highlight: true})
	}

	if plan.SortingKey != "" {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{Text: "sorting key: " + plan.SortingKey})
	}

	if plan.Parts != nil {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{
			Text: fmt.Sprintf("parts: %v  granules: %v", plan.Parts, plan.Granules),
		})
	}

	for _, idx := range plan.Indexes {
		text := fmt.Sprintf("index %s: %s", idx.Type, idx.Name)
		if idx.Condition != "" {
			text += " (" + idx.Condition + ")"
		}
		if idx.SelectedParts != nil && idx.InitialParts != nil {
			text += fmt.Sprintf("  parts %v/%v  granules %v/%v",
				idx.SelectedParts, idx.InitialParts,
				idx.SelectedGranules, idx.InitialGranules)
		}
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{Text: text})
	}

	for _, child := range plan.Plans {
		node.Children = append(node.Children, chPlanToNode(child))
	}

	return node
}

func chParseExplainText(raw string) plugin.BrowserExplainResult {
	lines := strings.Split(raw, "\n")
	root := plugin.BrowserExplainNode{Name: "Query Plan"}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		root.Lines = append(root.Lines, plugin.BrowserExplainLine{Text: line})
	}
	return plugin.BrowserExplainResult{Root: root}
}

// List navigates the ClickHouse hierarchy:
//
//	[]                                               -> databases
//	[database]                                       -> object type categories
//	[database, type]                                 -> objects of that type
//	[database, "table"|"view"|"matview", name]       -> sub-categories
//	[database, "table"|"view"|"matview", name, cat]  -> items in sub-category
func (b *ClickHouseBrowser) List(ctx context.Context, ids []string) ([]plugin.BrowserItem, error) {
	switch len(ids) {
	case 0:
		return b.listDatabases(ctx)
	case 1:
		return []plugin.BrowserItem{
			{ID: "table", Name: "Tables", HasChildren: true},
			{ID: "view", Name: "Views", HasChildren: true},
			{ID: "matview", Name: "Materialized Views", HasChildren: true},
			{ID: "dictionary", Name: "Dictionaries", HasChildren: true},
			{ID: "function", Name: "Functions", HasChildren: true},
		}, nil
	case 2:
		database, objectType := ids[0], ids[1]
		switch objectType {
		case "table":
			return b.listTables(ctx, database)
		case "view":
			return b.listViews(ctx, database)
		case "matview":
			return b.listMatViews(ctx, database)
		case "dictionary":
			return b.listDictionaries(ctx, database)
		case "function":
			return b.listFunctions(ctx, database)
		}
	case 3:
		objectType := ids[1]
		if objectType == "table" || objectType == "view" || objectType == "matview" {
			return []plugin.BrowserItem{
				{ID: "column", Name: "Columns", HasChildren: true},
				{ID: "index", Name: "Data Skipping Indexes", HasChildren: true},
			}, nil
		}
	case 4:
		database, objectType, tableName, category := ids[0], ids[1], ids[2], ids[3]
		if objectType == "table" || objectType == "view" || objectType == "matview" {
			switch category {
			case "column":
				return b.listColumns(ctx, database, tableName)
			case "index":
				return b.listIndexes(ctx, database, tableName)
			}
		}
	}
	return nil, fmt.Errorf("unsupported path depth: %d", len(ids))
}

func (b *ClickHouseBrowser) listDatabases(ctx context.Context) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT name FROM system.databases ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return chCollectItems(rows, true)
}

func (b *ClickHouseBrowser) listTables(ctx context.Context, database string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT name FROM system.tables WHERE database = ? AND engine != 'View' AND engine NOT LIKE '%MaterializedView%' ORDER BY name`,
		database,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return chCollectItems(rows, true)
}

func (b *ClickHouseBrowser) listViews(ctx context.Context, database string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT name FROM system.tables WHERE database = ? AND engine = 'View' ORDER BY name`,
		database,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return chCollectItems(rows, true)
}

func (b *ClickHouseBrowser) listMatViews(ctx context.Context, database string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT name FROM system.tables WHERE database = ? AND engine LIKE '%MaterializedView%' ORDER BY name`,
		database,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return chCollectItems(rows, true)
}

func (b *ClickHouseBrowser) listDictionaries(ctx context.Context, database string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT name FROM system.dictionaries WHERE database = ? ORDER BY name`,
		database,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return chCollectItems(rows, false)
}

func (b *ClickHouseBrowser) listFunctions(ctx context.Context, database string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT name FROM system.functions WHERE origin = 'SQLUserDefined' ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return chCollectItems(rows, false)
}

func (b *ClickHouseBrowser) listColumns(ctx context.Context, database, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT name, type, default_kind, default_expression, comment
		 FROM system.columns
		 WHERE database = ? AND table = ?
		 ORDER BY position`,
		database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []plugin.BrowserItem
	for rows.Next() {
		var name, colType, defaultKind, defaultExpr, comment string
		if err := rows.Scan(&name, &colType, &defaultKind, &defaultExpr, &comment); err != nil {
			return nil, err
		}
		label := name + " (" + colType + ")"
		if defaultKind != "" {
			label += " " + defaultKind
		}
		if comment != "" {
			label += " -- " + comment
		}
		items = append(items, plugin.BrowserItem{ID: name, Name: label, HasChildren: false})
	}
	return items, rows.Err()
}

func (b *ClickHouseBrowser) listIndexes(ctx context.Context, database, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT name, type, expr, granularity
		 FROM system.data_skipping_indices
		 WHERE database = ? AND table = ?
		 ORDER BY name`,
		database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []plugin.BrowserItem
	for rows.Next() {
		var name, idxType, expr string
		var granularity uint64
		if err := rows.Scan(&name, &idxType, &expr, &granularity); err != nil {
			return nil, err
		}
		label := fmt.Sprintf("%s (%s on %s, granularity %d)", name, idxType, expr, granularity)
		items = append(items, plugin.BrowserItem{ID: name, Name: label, HasChildren: false})
	}
	return items, rows.Err()
}

func chCollectItems(rows *sql.Rows, hasChildren bool) ([]plugin.BrowserItem, error) {
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
