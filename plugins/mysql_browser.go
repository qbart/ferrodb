package plugins

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qbart/ferrodb/ferro/config"
	"github.com/qbart/ferrodb/ferro/plugin"
)

type MySQLBrowser struct {
	driver *MySQLDriver
	conn   *MySQLDriverConnection
}

func NewMySQLBrowser(driver *MySQLDriver) *MySQLBrowser {
	return &MySQLBrowser{driver: driver}
}

func (b *MySQLBrowser) Connect(ctx context.Context, dsn string) error {
	conn, err := b.driver.Connect(ctx, config.DriverConfig{"dsn": dsn})
	if err != nil {
		return err
	}
	b.conn = conn.(*MySQLDriverConnection)
	return nil
}

func (b *MySQLBrowser) Disconnect(ctx context.Context) error {
	return b.driver.Disconnect(ctx, b.conn)
}

func (b *MySQLBrowser) Show(ctx context.Context, ids []string) (string, error) {
	if len(ids) < 3 {
		return "", nil
	}
	database, objectType := ids[0], ids[1]

	// table sub-item: [database, "table", table_name, category, item_name]
	if objectType == "table" && len(ids) == 5 {
		category, itemName := ids[3], ids[4]
		switch category {
		case "index":
			return fmt.Sprintf(
				`SELECT
    s.INDEX_NAME,
    s.COLUMN_NAME,
    s.SEQ_IN_INDEX,
    s.NON_UNIQUE,
    s.INDEX_TYPE,
    s.NULLABLE,
    s.CARDINALITY,
    s.SUB_PART,
    s.EXPRESSION,
    t.INDEX_TYPE AS CONSTRAINT_TYPE
FROM information_schema.STATISTICS s
LEFT JOIN information_schema.TABLE_CONSTRAINTS t
    ON t.TABLE_SCHEMA = s.TABLE_SCHEMA
    AND t.TABLE_NAME = s.TABLE_NAME
    AND t.CONSTRAINT_NAME = s.INDEX_NAME
WHERE s.TABLE_SCHEMA = '%s' AND s.INDEX_NAME = '%s'
ORDER BY s.SEQ_IN_INDEX`,
				database, itemName,
			), nil
		}
		return "", nil
	}

	name := ids[2]
	switch objectType {
	case "table", "view":
		return fmt.Sprintf("SELECT * FROM %s.%s LIMIT 100",
			mysqlQuoteIdent(database), mysqlQuoteIdent(name)), nil
	case "procedure":
		return fmt.Sprintf("SHOW CREATE PROCEDURE %s.%s",
			mysqlQuoteIdent(database), mysqlQuoteIdent(name)), nil
	case "function":
		return fmt.Sprintf("SHOW CREATE FUNCTION %s.%s",
			mysqlQuoteIdent(database), mysqlQuoteIdent(name)), nil
	case "event":
		return fmt.Sprintf("SHOW CREATE EVENT %s.%s",
			mysqlQuoteIdent(database), mysqlQuoteIdent(name)), nil
	}
	return "", nil
}

func mysqlTypeToEditorType(dbTypeName string) string {
	upper := strings.ToUpper(dbTypeName)
	switch {
	case upper == "JSON":
		return plugin.EditorTypeObject
	case strings.Contains(upper, "INT"):
		return plugin.EditorTypeInt64
	case strings.Contains(upper, "FLOAT"),
		strings.Contains(upper, "DOUBLE"),
		strings.Contains(upper, "DECIMAL"),
		strings.Contains(upper, "NUMERIC"):
		return plugin.EditorTypeFloat64
	default:
		return plugin.EditorTypeString
	}
}

func mysqlValueToString(v any) string {
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

func (b *MySQLBrowser) Query(ctx context.Context, query string) (plugin.BrowserQueryResult, error) {
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
		editorTypes[i] = mysqlTypeToEditorType(ct.DatabaseTypeName())
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
			row[i] = mysqlValueToString(v)
		}
		data = append(data, row)
	}
	if err := rows.Err(); err != nil {
		return plugin.BrowserQueryResult{}, err
	}

	return plugin.BrowserQueryResult{Headers: cols, Rows: data, ColumnTypes: editorTypes}, nil
}

// mysqlExplain* types model the MySQL EXPLAIN FORMAT=JSON output.

type mysqlExplainResult struct {
	QueryBlock mysqlExplainQueryBlock `json:"query_block"`
}

type mysqlExplainQueryBlock struct {
	SelectID          int                         `json:"select_id"`
	CostInfo          *mysqlExplainCostInfo       `json:"cost_info,omitempty"`
	Table             *mysqlExplainTableInfo      `json:"table,omitempty"`
	NestedLoop        []mysqlExplainNestedLoop    `json:"nested_loop,omitempty"`
	OrderingOperation *mysqlExplainOrderingOp     `json:"ordering_operation,omitempty"`
	GroupingOperation *mysqlExplainGroupingOp     `json:"grouping_operation,omitempty"`
	DuplicatesRemoval *mysqlExplainDuplicatesOp   `json:"duplicates_removal,omitempty"`
	Message           string                      `json:"message,omitempty"`
}

type mysqlExplainCostInfo struct {
	QueryCost       string `json:"query_cost,omitempty"`
	ReadCost        string `json:"read_cost,omitempty"`
	EvalCost        string `json:"eval_cost,omitempty"`
	PrefixCost      string `json:"prefix_cost,omitempty"`
	DataReadPerJoin string `json:"data_read_per_join,omitempty"`
	SortCost        string `json:"sort_cost,omitempty"`
}

type mysqlExplainTableInfo struct {
	TableName         string                `json:"table_name"`
	AccessType        string                `json:"access_type"`
	PossibleKeys      []string              `json:"possible_keys,omitempty"`
	Key               string                `json:"key,omitempty"`
	UsedKeyParts      []string              `json:"used_key_parts,omitempty"`
	KeyLength         string                `json:"key_length,omitempty"`
	Ref               []string              `json:"ref,omitempty"`
	RowsExamined      int64                 `json:"rows_examined_per_scan"`
	RowsProduced      int64                 `json:"rows_produced_per_join"`
	Filtered          string                `json:"filtered,omitempty"`
	UsingIndex        bool                  `json:"using_index,omitempty"`
	CostInfo          *mysqlExplainCostInfo `json:"cost_info,omitempty"`
	AttachedCondition string                `json:"attached_condition,omitempty"`
	UsedColumns       []string              `json:"used_columns,omitempty"`
}

type mysqlExplainNestedLoop struct {
	Table mysqlExplainTableInfo `json:"table"`
}

type mysqlExplainOrderingOp struct {
	UsingFilesort bool                       `json:"using_filesort"`
	UsingTmpTable bool                       `json:"using_temporary_table"`
	CostInfo      *mysqlExplainCostInfo      `json:"cost_info,omitempty"`
	Table         *mysqlExplainTableInfo     `json:"table,omitempty"`
	NestedLoop    []mysqlExplainNestedLoop   `json:"nested_loop,omitempty"`
	GroupingOp    *mysqlExplainGroupingOp    `json:"grouping_operation,omitempty"`
}

type mysqlExplainGroupingOp struct {
	UsingTmpTable bool                       `json:"using_temporary_table"`
	UsingFilesort bool                       `json:"using_filesort"`
	CostInfo      *mysqlExplainCostInfo      `json:"cost_info,omitempty"`
	Table         *mysqlExplainTableInfo     `json:"table,omitempty"`
	NestedLoop    []mysqlExplainNestedLoop   `json:"nested_loop,omitempty"`
}

type mysqlExplainDuplicatesOp struct {
	UsingTmpTable bool                       `json:"using_temporary_table"`
	CostInfo      *mysqlExplainCostInfo      `json:"cost_info,omitempty"`
	Table         *mysqlExplainTableInfo     `json:"table,omitempty"`
	NestedLoop    []mysqlExplainNestedLoop   `json:"nested_loop,omitempty"`
}

func (b *MySQLBrowser) ParseExplain(data plugin.BrowserQueryResult) (plugin.BrowserExplainResult, error) {
	if len(data.Rows) == 0 || len(data.Headers) == 0 {
		return plugin.BrowserExplainResult{}, fmt.Errorf("query result does not match the explain output")
	}
	var sb strings.Builder
	for _, row := range data.Rows {
		if len(row) > 0 {
			sb.WriteString(row[0])
		}
	}
	var raw mysqlExplainResult
	if err := json.Unmarshal([]byte(sb.String()), &raw); err != nil {
		return plugin.BrowserExplainResult{}, fmt.Errorf("query result does not match the explain output\n%s", err.Error())
	}

	qb := raw.QueryBlock

	var summary []plugin.BrowserExplainLine
	if qb.CostInfo != nil && qb.CostInfo.QueryCost != "" {
		summary = append(summary, plugin.BrowserExplainLine{
			Text: fmt.Sprintf("Query Cost: %s", qb.CostInfo.QueryCost),
		})
	}
	if qb.Message != "" {
		summary = append(summary, plugin.BrowserExplainLine{Text: qb.Message})
	}

	// Collect tables for per-table stats
	tables := mysqlCollectTables(qb)
	table := plugin.BrowserExplainTable{
		Title:   "Per table stats",
		Headers: []string{"table", "access", "key", "rows examined", "rows produced", "filtered"},
	}
	for _, t := range tables {
		filtered := t.Filtered
		if filtered == "" {
			filtered = "100.00"
		}
		highlight := t.AccessType == "ALL" || t.AccessType == "index"
		table.Rows = append(table.Rows, plugin.BrowserExplainRow{
			Cells: []string{
				t.TableName,
				t.AccessType,
				t.Key,
				fmt.Sprintf("%d", t.RowsExamined),
				fmt.Sprintf("%d", t.RowsProduced),
				filtered + "%",
			},
			Highlight: highlight,
		})
	}

	return plugin.BrowserExplainResult{
		Root:         mysqlBuildExplainTree(qb),
		SummaryLines: summary,
		Tables:       []plugin.BrowserExplainTable{table},
	}, nil
}

func mysqlCollectTables(qb mysqlExplainQueryBlock) []mysqlExplainTableInfo {
	var tables []mysqlExplainTableInfo
	if qb.Table != nil {
		tables = append(tables, *qb.Table)
	}
	for _, nl := range qb.NestedLoop {
		tables = append(tables, nl.Table)
	}
	if qb.OrderingOperation != nil {
		tables = append(tables, mysqlCollectTablesFromOrdering(qb.OrderingOperation)...)
	}
	if qb.GroupingOperation != nil {
		tables = append(tables, mysqlCollectTablesFromGrouping(qb.GroupingOperation)...)
	}
	if qb.DuplicatesRemoval != nil {
		if qb.DuplicatesRemoval.Table != nil {
			tables = append(tables, *qb.DuplicatesRemoval.Table)
		}
		for _, nl := range qb.DuplicatesRemoval.NestedLoop {
			tables = append(tables, nl.Table)
		}
	}
	return tables
}

func mysqlCollectTablesFromOrdering(op *mysqlExplainOrderingOp) []mysqlExplainTableInfo {
	var tables []mysqlExplainTableInfo
	if op.Table != nil {
		tables = append(tables, *op.Table)
	}
	for _, nl := range op.NestedLoop {
		tables = append(tables, nl.Table)
	}
	if op.GroupingOp != nil {
		tables = append(tables, mysqlCollectTablesFromGrouping(op.GroupingOp)...)
	}
	return tables
}

func mysqlCollectTablesFromGrouping(op *mysqlExplainGroupingOp) []mysqlExplainTableInfo {
	var tables []mysqlExplainTableInfo
	if op.Table != nil {
		tables = append(tables, *op.Table)
	}
	for _, nl := range op.NestedLoop {
		tables = append(tables, nl.Table)
	}
	return tables
}

func mysqlBuildExplainTree(qb mysqlExplainQueryBlock) plugin.BrowserExplainNode {
	root := plugin.BrowserExplainNode{Name: "Query Block"}

	if qb.CostInfo != nil && qb.CostInfo.QueryCost != "" {
		root.Lines = append(root.Lines, plugin.BrowserExplainLine{
			Text: "cost " + qb.CostInfo.QueryCost,
		})
	}

	if qb.OrderingOperation != nil {
		node := mysqlOrderingNode(qb.OrderingOperation)
		root.Children = append(root.Children, node)
	} else if qb.GroupingOperation != nil {
		node := mysqlGroupingNode(qb.GroupingOperation)
		root.Children = append(root.Children, node)
	} else if qb.DuplicatesRemoval != nil {
		node := mysqlDuplicatesNode(qb.DuplicatesRemoval)
		root.Children = append(root.Children, node)
	} else if len(qb.NestedLoop) > 0 {
		for _, nl := range qb.NestedLoop {
			root.Children = append(root.Children, mysqlTableNode(&nl.Table))
		}
	} else if qb.Table != nil {
		root.Children = append(root.Children, mysqlTableNode(qb.Table))
	}

	if qb.Message != "" {
		root.Lines = append(root.Lines, plugin.BrowserExplainLine{Text: qb.Message})
	}

	return root
}

func mysqlTableNode(t *mysqlExplainTableInfo) plugin.BrowserExplainNode {
	name := t.AccessType
	if t.TableName != "" {
		name += " on " + t.TableName
	}
	node := plugin.BrowserExplainNode{Name: name}

	if t.Key != "" {
		keyInfo := "key: " + t.Key
		if len(t.UsedKeyParts) > 0 {
			keyInfo += " (" + strings.Join(t.UsedKeyParts, ", ") + ")"
		}
		if t.KeyLength != "" {
			keyInfo += " len=" + t.KeyLength
		}
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{Text: keyInfo})
	}

	if len(t.Ref) > 0 {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{
			Text: "ref: " + strings.Join(t.Ref, ", "),
		})
	}

	node.Lines = append(node.Lines, plugin.BrowserExplainLine{
		Text: fmt.Sprintf("rows examined %d  rows produced %d", t.RowsExamined, t.RowsProduced),
	})

	if t.Filtered != "" {
		fVal, _ := strconv.ParseFloat(t.Filtered, 64)
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{
			Text:      fmt.Sprintf("filtered %.2f%%", fVal),
			Highlight: fVal < 50,
		})
	}

	if t.CostInfo != nil {
		var parts []string
		if t.CostInfo.ReadCost != "" {
			parts = append(parts, "read="+t.CostInfo.ReadCost)
		}
		if t.CostInfo.EvalCost != "" {
			parts = append(parts, "eval="+t.CostInfo.EvalCost)
		}
		if t.CostInfo.PrefixCost != "" {
			parts = append(parts, "prefix="+t.CostInfo.PrefixCost)
		}
		if t.CostInfo.DataReadPerJoin != "" {
			parts = append(parts, "data read="+t.CostInfo.DataReadPerJoin)
		}
		if len(parts) > 0 {
			node.Lines = append(node.Lines, plugin.BrowserExplainLine{
				Text: "cost: " + strings.Join(parts, "  "),
			})
		}
	}

	if t.AttachedCondition != "" {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{
			Text:      "filter: " + t.AttachedCondition,
			Highlight: true,
		})
	}

	if t.UsingIndex {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{Text: "using index (covering)"})
	}

	if t.AccessType == "ALL" {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{
			Text:      "full table scan",
			Highlight: true,
		})
	}

	return node
}

func mysqlOrderingNode(op *mysqlExplainOrderingOp) plugin.BrowserExplainNode {
	name := "Ordering"
	if op.UsingFilesort {
		name += " (filesort)"
	}
	node := plugin.BrowserExplainNode{
		Name: name,
		Lines: []plugin.BrowserExplainLine{
			{Text: fmt.Sprintf("using filesort: %v", op.UsingFilesort), Highlight: op.UsingFilesort},
		},
	}
	if op.CostInfo != nil && op.CostInfo.SortCost != "" {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{
			Text: "sort cost: " + op.CostInfo.SortCost,
		})
	}

	if op.GroupingOp != nil {
		node.Children = append(node.Children, mysqlGroupingNode(op.GroupingOp))
	} else if len(op.NestedLoop) > 0 {
		for _, nl := range op.NestedLoop {
			node.Children = append(node.Children, mysqlTableNode(&nl.Table))
		}
	} else if op.Table != nil {
		node.Children = append(node.Children, mysqlTableNode(op.Table))
	}

	return node
}

func mysqlGroupingNode(op *mysqlExplainGroupingOp) plugin.BrowserExplainNode {
	name := "Grouping"
	if op.UsingTmpTable {
		name += " (tmp table)"
	}
	node := plugin.BrowserExplainNode{Name: name}
	if op.UsingTmpTable {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{
			Text: "using temporary table", Highlight: true,
		})
	}
	if op.UsingFilesort {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{
			Text: "using filesort", Highlight: true,
		})
	}

	if len(op.NestedLoop) > 0 {
		for _, nl := range op.NestedLoop {
			node.Children = append(node.Children, mysqlTableNode(&nl.Table))
		}
	} else if op.Table != nil {
		node.Children = append(node.Children, mysqlTableNode(op.Table))
	}

	return node
}

func mysqlDuplicatesNode(op *mysqlExplainDuplicatesOp) plugin.BrowserExplainNode {
	node := plugin.BrowserExplainNode{Name: "Duplicates Removal"}
	if op.UsingTmpTable {
		node.Lines = append(node.Lines, plugin.BrowserExplainLine{
			Text: "using temporary table", Highlight: true,
		})
	}

	if len(op.NestedLoop) > 0 {
		for _, nl := range op.NestedLoop {
			node.Children = append(node.Children, mysqlTableNode(&nl.Table))
		}
	} else if op.Table != nil {
		node.Children = append(node.Children, mysqlTableNode(op.Table))
	}

	return node
}

// List navigates the MySQL database hierarchy:
//
//	[]                                       → databases
//	[database]                               → object type categories
//	[database, type]                         → objects of that type
//	[database, "table", name]                → table sub-categories
//	[database, "table", name, category]      → items in sub-category
func (b *MySQLBrowser) List(ctx context.Context, ids []string) ([]plugin.BrowserItem, error) {
	switch len(ids) {
	case 0:
		return b.listDatabases(ctx)
	case 1:
		return []plugin.BrowserItem{
			{ID: "table", Name: "Tables", HasChildren: true},
			{ID: "view", Name: "Views", HasChildren: true},
			{ID: "procedure", Name: "Procedures", HasChildren: true},
			{ID: "function", Name: "Functions", HasChildren: true},
			{ID: "trigger", Name: "Triggers", HasChildren: true},
			{ID: "event", Name: "Events", HasChildren: true},
		}, nil
	case 2:
		database, objectType := ids[0], ids[1]
		switch objectType {
		case "table":
			return b.listTables(ctx, database)
		case "view":
			return b.listViews(ctx, database)
		case "procedure":
			return b.listProcedures(ctx, database)
		case "function":
			return b.listFunctions(ctx, database)
		case "trigger":
			return b.listTriggers(ctx, database)
		case "event":
			return b.listEvents(ctx, database)
		}
	case 3:
		objectType := ids[1]
		if objectType == "table" {
			return []plugin.BrowserItem{
				{ID: "column", Name: "Columns", HasChildren: true},
				{ID: "index", Name: "Indexes", HasChildren: true},
				{ID: "constraint", Name: "Constraints", HasChildren: true},
				{ID: "foreign_key", Name: "Foreign Keys", HasChildren: true},
				{ID: "trigger", Name: "Triggers", HasChildren: true},
			}, nil
		}
	case 4:
		database, objectType, tableName, category := ids[0], ids[1], ids[2], ids[3]
		if objectType == "table" {
			switch category {
			case "column":
				return b.listColumns(ctx, database, tableName)
			case "index":
				return b.listIndexes(ctx, database, tableName)
			case "constraint":
				return b.listConstraints(ctx, database, tableName)
			case "foreign_key":
				return b.listForeignKeys(ctx, database, tableName)
			case "trigger":
				return b.listTableTriggers(ctx, database, tableName)
			}
		}
	}
	return nil, fmt.Errorf("unsupported path depth: %d", len(ids))
}

func (b *MySQLBrowser) listDatabases(ctx context.Context) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT schema_name FROM information_schema.schemata ORDER BY schema_name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return mysqlCollectItems(rows, true)
}

func (b *MySQLBrowser) listTables(ctx context.Context, database string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE' ORDER BY table_name`,
		database,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return mysqlCollectItems(rows, true)
}

func (b *MySQLBrowser) listViews(ctx context.Context, database string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT table_name FROM information_schema.views WHERE table_schema = ? ORDER BY table_name`,
		database,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return mysqlCollectItems(rows, false)
}

func (b *MySQLBrowser) listProcedures(ctx context.Context, database string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT routine_name FROM information_schema.routines WHERE routine_schema = ? AND routine_type = 'PROCEDURE' ORDER BY routine_name`,
		database,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return mysqlCollectItems(rows, false)
}

func (b *MySQLBrowser) listFunctions(ctx context.Context, database string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT routine_name FROM information_schema.routines WHERE routine_schema = ? AND routine_type = 'FUNCTION' ORDER BY routine_name`,
		database,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return mysqlCollectItems(rows, false)
}

func (b *MySQLBrowser) listTriggers(ctx context.Context, database string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT DISTINCT trigger_name FROM information_schema.triggers WHERE trigger_schema = ? ORDER BY trigger_name`,
		database,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return mysqlCollectItems(rows, false)
}

func (b *MySQLBrowser) listEvents(ctx context.Context, database string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT event_name FROM information_schema.events WHERE event_schema = ? ORDER BY event_name`,
		database,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return mysqlCollectItems(rows, false)
}

func (b *MySQLBrowser) listColumns(ctx context.Context, database, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT column_name, column_type, is_nullable, column_key
		 FROM information_schema.columns
		 WHERE table_schema = ? AND table_name = ?
		 ORDER BY ordinal_position`,
		database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []plugin.BrowserItem
	for rows.Next() {
		var name, colType, nullable, key string
		if err := rows.Scan(&name, &colType, &nullable, &key); err != nil {
			return nil, err
		}
		label := name + " (" + colType + ")"
		if key == "PRI" {
			label += " PK"
		} else if key == "UNI" {
			label += " UNIQUE"
		}
		if nullable == "NO" && key != "PRI" {
			label += " NOT NULL"
		}
		items = append(items, plugin.BrowserItem{ID: name, Name: label, HasChildren: false})
	}
	return items, rows.Err()
}

func (b *MySQLBrowser) listIndexes(ctx context.Context, database, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT DISTINCT index_name, non_unique
		 FROM information_schema.statistics
		 WHERE table_schema = ? AND table_name = ?
		 ORDER BY index_name`,
		database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []plugin.BrowserItem
	for rows.Next() {
		var name string
		var nonUnique int
		if err := rows.Scan(&name, &nonUnique); err != nil {
			return nil, err
		}
		label := name
		if name == "PRIMARY" {
			label += " (primary key)"
		} else if nonUnique == 0 {
			label += " (unique)"
		}
		items = append(items, plugin.BrowserItem{ID: name, Name: label, HasChildren: false})
	}
	return items, rows.Err()
}

func (b *MySQLBrowser) listConstraints(ctx context.Context, database, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT constraint_name, constraint_type
		 FROM information_schema.table_constraints
		 WHERE table_schema = ? AND table_name = ?
		 ORDER BY constraint_name`,
		database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []plugin.BrowserItem
	for rows.Next() {
		var name, cType string
		if err := rows.Scan(&name, &cType); err != nil {
			return nil, err
		}
		label := name + " (" + cType + ")"
		items = append(items, plugin.BrowserItem{ID: name, Name: label, HasChildren: false})
	}
	return items, rows.Err()
}

func (b *MySQLBrowser) listForeignKeys(ctx context.Context, database, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT
			kcu.CONSTRAINT_NAME,
			kcu.COLUMN_NAME,
			kcu.REFERENCED_TABLE_NAME,
			kcu.REFERENCED_COLUMN_NAME
		 FROM information_schema.KEY_COLUMN_USAGE kcu
		 WHERE kcu.TABLE_SCHEMA = ? AND kcu.TABLE_NAME = ?
		   AND kcu.REFERENCED_TABLE_NAME IS NOT NULL
		 ORDER BY kcu.CONSTRAINT_NAME, kcu.ORDINAL_POSITION`,
		database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []plugin.BrowserItem
	for rows.Next() {
		var constraintName, colName, refTable, refCol string
		if err := rows.Scan(&constraintName, &colName, &refTable, &refCol); err != nil {
			return nil, err
		}
		label := fmt.Sprintf("%s: %s → %s(%s)", constraintName, colName, refTable, refCol)
		items = append(items, plugin.BrowserItem{ID: constraintName, Name: label, HasChildren: false})
	}
	return items, rows.Err()
}

func (b *MySQLBrowser) listTableTriggers(ctx context.Context, database, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.DB.QueryContext(ctx,
		`SELECT trigger_name FROM information_schema.triggers
		 WHERE trigger_schema = ? AND event_object_table = ?
		 ORDER BY trigger_name`,
		database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return mysqlCollectItems(rows, false)
}

func mysqlCollectItems(rows *sql.Rows, hasChildren bool) ([]plugin.BrowserItem, error) {
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
