package plugins

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/qbart/ferrodb/ferro/config"
	"github.com/qbart/ferrodb/ferro/plugin"
)

type SQLiteBrowser struct {
	driver *SQLiteDriver
	conn   *SQLiteDriverConnection
}

func NewSQLiteBrowser(driver *SQLiteDriver) *SQLiteBrowser {
	return &SQLiteBrowser{driver: driver}
}

func (b *SQLiteBrowser) Connect(ctx context.Context, dsn string) error {
	conn, err := b.driver.Connect(ctx, config.DriverConfig{"path": dsn})
	if err != nil {
		return err
	}
	b.conn = conn.(*SQLiteDriverConnection)
	return nil
}

func (b *SQLiteBrowser) Disconnect(ctx context.Context) error {
	return b.driver.Disconnect(ctx, b.conn)
}

// List navigates a hierarchy with no schema level (SQLite is schema-free):
//
//	[]            → object type categories (Tables, Views, Triggers)
//	[type]        → objects of that type
//	[table, name] → sub-categories (Columns, Indexes, Foreign Keys, Triggers)
//	[table, name, category] → items within that category
func (b *SQLiteBrowser) List(ctx context.Context, ids []string) ([]plugin.BrowserItem, error) {
	switch len(ids) {
	case 0:
		return []plugin.BrowserItem{
			{ID: "table", Name: "Tables", HasChildren: true},
			{ID: "view", Name: "Views", HasChildren: true},
			{ID: "trigger", Name: "Triggers", HasChildren: true},
		}, nil

	case 1:
		switch ids[0] {
		case "table":
			return b.listTables(ctx)
		case "view":
			return b.listViews(ctx)
		case "trigger":
			return b.listTriggers(ctx)
		}

	case 2:
		if ids[0] == "table" {
			return b.listTableCategories(ctx, ids[1])
		}

	case 3:
		objectType, tableName, category := ids[0], ids[1], ids[2]
		if objectType == "table" {
			switch category {
			case "column":
				return b.listColumns(ctx, tableName)
			case "index":
				return b.listIndexes(ctx, tableName)
			case "foreign_key":
				return b.listForeignKeys(ctx, tableName)
			case "trigger":
				return b.listTableTriggers(ctx, tableName)
			}
		}
	}
	return nil, fmt.Errorf("unsupported path depth: %d", len(ids))
}

// Show returns a query that previews the selected object.
//
//	["table"|"view", name]            → SELECT * FROM name LIMIT 100
//	["trigger", name]                 → SELECT sql FROM sqlite_master …
//	["table", name, "index", idx]     → SELECT sql FROM sqlite_master …
func (b *SQLiteBrowser) Show(ctx context.Context, ids []string) (string, error) {
	if len(ids) < 2 {
		return "", nil
	}
	objectType, name := ids[0], ids[1]

	// Sub-item: ["table", tableName, category, itemName]
	if objectType == "table" && len(ids) == 4 {
		category, itemName := ids[2], ids[3]
		switch category {
		case "index":
			return fmt.Sprintf(
				`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = %s`,
				sqliteLiteralStr(itemName),
			), nil
		}
		return "", nil
	}

	switch objectType {
	case "table", "view":
		return fmt.Sprintf("SELECT * FROM %s LIMIT 100", sqliteQuoteIdent(name)), nil
	case "trigger":
		return fmt.Sprintf(
			`SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = %s`,
			sqliteLiteralStr(name),
		), nil
	}
	return "", nil
}

// Query executes arbitrary SQL against the database using the read pool.
func (b *SQLiteBrowser) Query(ctx context.Context, query string) (plugin.BrowserQueryResult, error) {
	rows, err := b.conn.Read.QueryContext(ctx, query)
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
		editorTypes[i] = sqliteTypeToEditorType(ct.DatabaseTypeName())
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
			row[i] = sqliteBrowserValueToString(v)
		}
		data = append(data, row)
	}
	if err := rows.Err(); err != nil {
		return plugin.BrowserQueryResult{}, err
	}

	return plugin.BrowserQueryResult{Headers: cols, Rows: data, ColumnTypes: editorTypes}, nil
}

// ParseExplain parses the output of SQLite EXPLAIN (VDBE bytecode).
//
// SQLite EXPLAIN produces 8 columns: addr opcode p1 p2 p3 p4 p5 comment.
// The result contains:
//   - A full bytecode table with key opcodes highlighted.
//   - A cursor access table derived from OpenRead/OpenWrite instructions.
//   - A root node summarising the key operation counts.
func (b *SQLiteBrowser) ParseExplain(data plugin.BrowserQueryResult) (plugin.BrowserExplainResult, error) {
	if len(data.Headers) != 8 || len(data.Rows) == 0 {
		return plugin.BrowserExplainResult{}, fmt.Errorf(
			"query result does not match SQLite EXPLAIN output (expected columns: addr opcode p1 p2 p3 p4 p5 comment)",
		)
	}

	type cursorEntry struct {
		cursor   string
		access   string
		name     string
		rootPage string
	}

	var bytecodeRows []plugin.BrowserExplainRow
	var cursors []cursorEntry
	cursorSeen := make(map[string]bool)
	opcodeCounts := make(map[string]int)

	for _, row := range data.Rows {
		if len(row) != 8 {
			continue
		}
		opcode := row[1]
		opcodeCounts[opcode]++

		bytecodeRows = append(bytecodeRows, plugin.BrowserExplainRow{
			Cells:     row,
			Highlight: sqliteIsKeyOpcode(opcode),
		})

		// Build cursor access summary from OpenRead / OpenWrite instructions.
		// VDBE layout: addr opcode p1(cursor) p2(root-page) p3 p4(name) p5 comment
		if opcode == "OpenRead" || opcode == "OpenWrite" {
			cursor := row[2]
			if !cursorSeen[cursor] {
				cursorSeen[cursor] = true
				access := "Read"
				if opcode == "OpenWrite" {
					access = "Write"
				}
				name := row[5] // p4 holds the table/index name
				if name == "" {
					name = "?"
				}
				cursors = append(cursors, cursorEntry{
					cursor:   cursor,
					access:   access,
					name:     name,
					rootPage: row[3], // p2
				})
			}
		}
	}

	// Root node: counts of the most interesting opcodes.
	root := plugin.BrowserExplainNode{Name: "SQLite VDBE"}
	for _, op := range []string{
		"OpenRead", "OpenWrite",
		"Rewind", "Next", "Prev",
		"SeekRowid", "SeekGE", "SeekGT", "SeekLE", "SeekLT",
		"IdxGE", "IdxGT", "IdxLE", "IdxLT",
		"ResultRow",
	} {
		if n, ok := opcodeCounts[op]; ok {
			root.Lines = append(root.Lines, plugin.BrowserExplainLine{
				Text:      fmt.Sprintf("%-16s %d", op, n),
				Highlight: strings.HasPrefix(op, "Open") && n > 0,
			})
		}
	}

	summary := []plugin.BrowserExplainLine{
		{Text: fmt.Sprintf("Instructions: %d", len(bytecodeRows))},
	}

	// Table 1: full bytecode listing.
	bytecodeTable := plugin.BrowserExplainTable{
		Title:   "VDBE Bytecode",
		Headers: data.Headers,
		Rows:    bytecodeRows,
	}
	tables := []plugin.BrowserExplainTable{bytecodeTable}

	// Table 2: cursor access summary (only when cursors were found).
	if len(cursors) > 0 {
		cursorTable := plugin.BrowserExplainTable{
			Title:   "Cursor Access",
			Headers: []string{"cursor", "access", "table / index", "root page"},
		}
		for _, c := range cursors {
			cursorTable.Rows = append(cursorTable.Rows, plugin.BrowserExplainRow{
				Cells:     []string{c.cursor, c.access, c.name, c.rootPage},
				Highlight: c.access == "Write",
			})
		}
		tables = append(tables, cursorTable)
	}

	return plugin.BrowserExplainResult{
		Root:         root,
		SummaryLines: summary,
		Tables:       tables,
	}, nil
}

// sqliteIsKeyOpcode returns true for opcodes that are significant for
// understanding query execution (cursor opens, scans, seeks, output).
func sqliteIsKeyOpcode(op string) bool {
	switch op {
	case "OpenRead", "OpenWrite", "OpenEphemeral", "OpenAutoindex",
		"TableLock",
		"Rewind", "Next", "Prev", "NextIfOpen",
		"SeekGE", "SeekGT", "SeekLE", "SeekLT", "SeekRowid",
		"IdxGE", "IdxGT", "IdxLE", "IdxLT",
		"Found", "NotFound", "NotExists",
		"ResultRow", "Halt":
		return true
	}
	return false
}

// --- list helpers ---

func (b *SQLiteBrowser) listTables(ctx context.Context) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Read.QueryContext(ctx,
		`SELECT name FROM sqlite_master
		 WHERE type = 'table'
		   AND name NOT LIKE 'sqlite_%'
		   AND name NOT LIKE '_ferro_%'
		 ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return sqliteCollectItems(rows, true)
}

func (b *SQLiteBrowser) listViews(ctx context.Context) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Read.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'view' ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return sqliteCollectItems(rows, false)
}

func (b *SQLiteBrowser) listTriggers(ctx context.Context) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Read.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'trigger' ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return sqliteCollectItems(rows, false)
}

func (b *SQLiteBrowser) listTableCategories(_ context.Context, _ string) ([]plugin.BrowserItem, error) {
	return []plugin.BrowserItem{
		{ID: "column", Name: "Columns", HasChildren: true},
		{ID: "index", Name: "Indexes", HasChildren: true},
		{ID: "foreign_key", Name: "Foreign Keys", HasChildren: true},
		{ID: "trigger", Name: "Triggers", HasChildren: true},
	}, nil
}

// listColumns uses PRAGMA table_info which returns:
// cid, name, type, notnull, dflt_value, pk
func (b *SQLiteBrowser) listColumns(ctx context.Context, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Read.QueryContext(ctx,
		fmt.Sprintf("PRAGMA table_info(%s)", sqliteQuoteIdent(table)),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []plugin.BrowserItem
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		label := name
		if typ != "" {
			label += " (" + typ + ")"
		}
		if pk > 0 {
			label += " PK"
		}
		if notNull == 1 && pk == 0 {
			label += " NOT NULL"
		}
		items = append(items, plugin.BrowserItem{ID: name, Name: label, HasChildren: false})
	}
	return items, rows.Err()
}

// listIndexes uses PRAGMA index_list which returns:
// seq, name, unique, origin, partial
func (b *SQLiteBrowser) listIndexes(ctx context.Context, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Read.QueryContext(ctx,
		fmt.Sprintf("PRAGMA index_list(%s)", sqliteQuoteIdent(table)),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []plugin.BrowserItem
	for rows.Next() {
		var seq, unique, partial int
		var name, origin string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return nil, err
		}
		label := name
		switch {
		case unique == 1 && origin == "pk":
			label += " (primary key)"
		case unique == 1:
			label += " (unique)"
		case partial == 1:
			label += " (partial)"
		}
		items = append(items, plugin.BrowserItem{ID: name, Name: label, HasChildren: false})
	}
	return items, rows.Err()
}

// listForeignKeys uses PRAGMA foreign_key_list which returns:
// id, seq, table, from, to, on_update, on_delete, match
func (b *SQLiteBrowser) listForeignKeys(ctx context.Context, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Read.QueryContext(ctx,
		fmt.Sprintf("PRAGMA foreign_key_list(%s)", sqliteQuoteIdent(table)),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []plugin.BrowserItem
	for rows.Next() {
		var id, seq int
		var refTable, fromCol, toCol, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &refTable, &fromCol, &toCol, &onUpdate, &onDelete, &match); err != nil {
			return nil, err
		}
		itemID := fmt.Sprintf("%d", id)
		label := fmt.Sprintf("%s → %s(%s)", fromCol, refTable, toCol)
		items = append(items, plugin.BrowserItem{ID: itemID, Name: label, HasChildren: false})
	}
	return items, rows.Err()
}

func (b *SQLiteBrowser) listTableTriggers(ctx context.Context, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Read.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'trigger' AND tbl_name = ? ORDER BY name`,
		table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return sqliteCollectItems(rows, false)
}

// --- shared helpers ---

func sqliteCollectItems(rows *sql.Rows, hasChildren bool) ([]plugin.BrowserItem, error) {
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

// sqliteTypeToEditorType maps SQLite type affinity names to editor types.
func sqliteTypeToEditorType(dbTypeName string) string {
	upper := strings.ToUpper(dbTypeName)
	switch {
	case strings.Contains(upper, "INT"):
		return plugin.EditorTypeInt64
	case strings.Contains(upper, "REAL"),
		strings.Contains(upper, "FLOA"),
		strings.Contains(upper, "DOUB"):
		return plugin.EditorTypeFloat64
	case strings.Contains(upper, "NUM"),
		strings.Contains(upper, "DEC"):
		return plugin.EditorTypeFloat64
	default:
		return plugin.EditorTypeString
	}
}

// sqliteBrowserValueToString converts a scanned sql value to a display string.
func sqliteBrowserValueToString(v any) string {
	if v == nil {
		return "NULL"
	}
	if b, ok := v.([]byte); ok {
		if utf8.Valid(b) {
			return string(b)
		}
		return "0x" + hex.EncodeToString(b)
	}
	return fmt.Sprintf("%v", v)
}

// sqliteLiteralStr wraps s in single quotes, escaping internal single quotes.
func sqliteLiteralStr(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
