package plugins

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
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
//
// Result layout:
//   - Root: logical plan tree derived from opcode patterns (scans, filters,
//     projections, global aggregation/sort/limit).
//   - SummaryLines: instruction count and key opcode tallies.
//   - Tables[0]: full VDBE bytecode listing with key opcodes highlighted.
//   - Tables[1]: cursor access summary (OpenRead / OpenWrite).
func (b *SQLiteBrowser) ParseExplain(data plugin.BrowserQueryResult) (plugin.BrowserExplainResult, error) {
	if len(data.Headers) != 8 || len(data.Rows) == 0 {
		return plugin.BrowserExplainResult{}, fmt.Errorf(
			"query result does not match SQLite EXPLAIN output (expected columns: addr opcode p1 p2 p3 p4 p5 comment)",
		)
	}

	var bytecodeRows []plugin.BrowserExplainRow
	opcodeCounts := make(map[string]int)

	for _, row := range data.Rows {
		if len(row) != 8 {
			continue
		}
		opcodeCounts[row[1]]++
		bytecodeRows = append(bytecodeRows, plugin.BrowserExplainRow{
			Cells:     row,
			Highlight: sqliteIsKeyOpcode(row[1]),
		})
	}

	// Parse instructions and resolve schema names before building any output.
	instrs := parseSQLiteVDBE(data.Rows)
	schema := b.buildVDBESchema(instrs)

	// Build Cursor Access table using schema-resolved names.
	type cursorEntry struct {
		cursor   string
		access   string
		name     string
		rootPage string
	}
	var cursorEntries []cursorEntry
	cursorSeen := make(map[string]bool)
	for _, row := range data.Rows {
		if len(row) != 8 {
			continue
		}
		opcode := row[1]
		if opcode != "OpenRead" && opcode != "OpenWrite" {
			continue
		}
		cursor := row[2]
		if cursorSeen[cursor] {
			continue
		}
		cursorSeen[cursor] = true
		access := "Read"
		if opcode == "OpenWrite" {
			access = "Write"
		}
		cursorID, _ := strconv.Atoi(cursor)
		cursorEntries = append(cursorEntries, cursorEntry{
			cursor:   cursor,
			access:   access,
			name:     schema.tableName(cursorID),
			rootPage: row[3],
		})
	}

	summary := []plugin.BrowserExplainLine{
		{Text: fmt.Sprintf("Instructions: %d", len(bytecodeRows))},
	}
	for _, op := range []string{
		"OpenRead", "OpenWrite",
		"Rewind", "Next",
		"SeekRowid", "SeekGE", "SeekGT", "SeekLE", "SeekLT",
		"ResultRow",
	} {
		if n := opcodeCounts[op]; n > 0 {
			summary = append(summary, plugin.BrowserExplainLine{
				Text: fmt.Sprintf("%-16s %d", op, n),
			})
		}
	}

	bytecodeTable := plugin.BrowserExplainTable{
		Title:   "VDBE Bytecode",
		Headers: data.Headers,
		Rows:    bytecodeRows,
	}
	tables := []plugin.BrowserExplainTable{bytecodeTable}

	if len(cursorEntries) > 0 {
		cursorTable := plugin.BrowserExplainTable{
			Title:   "Cursor Access",
			Headers: []string{"cursor", "access", "table / index", "root page"},
		}
		for _, c := range cursorEntries {
			cursorTable.Rows = append(cursorTable.Rows, plugin.BrowserExplainRow{
				Cells:     []string{c.cursor, c.access, c.name, c.rootPage},
				Highlight: c.access == "Write",
			})
		}
		tables = append(tables, cursorTable)
	}

	return plugin.BrowserExplainResult{
		Root:         buildSQLiteLogicalPlan(instrs, schema),
		SummaryLines: summary,
		Tables:       tables,
	}, nil
}

// vdbeInstr is a single decoded row from SQLite EXPLAIN output.
type vdbeInstr struct {
	addr    int
	opcode  string
	p1      int    // cursor id, register, or first operand
	p2      int    // jump target, first arg register (Function), or second operand
	p3      int    // result register (Column / Function) or third operand
	p4      string // text operand: table/index name, function signature, etc.
	comment string // human-readable annotation from SQLite EXPLAIN
}

// parseSQLiteVDBE converts raw string rows from EXPLAIN into vdbeInstrs.
// Row layout: addr opcode p1 p2 p3 p4 p5 comment (8 columns).
func parseSQLiteVDBE(rows [][]string) []vdbeInstr {
	out := make([]vdbeInstr, 0, len(rows))
	for _, row := range rows {
		if len(row) != 8 {
			continue
		}
		addr, _ := strconv.Atoi(row[0])
		p1, _ := strconv.Atoi(row[2])
		p2, _ := strconv.Atoi(row[3])
		p3, _ := strconv.Atoi(row[4])
		out = append(out, vdbeInstr{
			addr: addr, opcode: row[1],
			p1: p1, p2: p2, p3: p3,
			p4: row[5], comment: row[7],
		})
	}
	return out
}

// vdbeNameFromComment extracts the table or index name from a SQLite EXPLAIN
// comment. SQLite with EXPLAIN_COMMENTS enabled emits text like
// "root=2 iDb=0; albums" — the name follows the last semicolon.
func vdbeNameFromComment(comment string) string {
	if i := strings.LastIndex(comment, ";"); i >= 0 {
		name := strings.TrimSpace(comment[i+1:])
		if name != "" {
			return name
		}
	}
	return ""
}

// vdbeSchema carries table and column names resolved from the live database,
// keyed by cursor id and column index.
type vdbeSchema struct {
	cursorTable  map[int]string   // cursor id → table name
	tableColumns map[string][]string // table name → ordered column names (by cid)
}

// tableName returns the table name for a cursor, or "?" if unknown.
func (s *vdbeSchema) tableName(cursor int) string {
	if s == nil {
		return "?"
	}
	if n, ok := s.cursorTable[cursor]; ok {
		return n
	}
	return "?"
}

// colRef returns "table.ColumnName" for a (cursor, col-index) pair.
func (s *vdbeSchema) colRef(cursor, col int) string {
	if s == nil {
		return fmt.Sprintf("col[%d]", col)
	}
	tbl := s.tableName(cursor)
	if tbl == "?" {
		return fmt.Sprintf("col[%d]", col)
	}
	cols := s.tableColumns[tbl]
	if col < len(cols) && cols[col] != "" {
		return tbl + "." + cols[col]
	}
	return fmt.Sprintf("%s.col[%d]", tbl, col)
}

// buildVDBESchema queries the connected database to resolve cursor ids to
// table names and column lists. It uses two strategies in order:
//  1. Parse the SQLite EXPLAIN comment field (available when SQLite is built
//     with SQLITE_ENABLE_EXPLAIN_COMMENTS). Comments look like
//     "root=2 iDb=0; tablename".
//  2. Look up the root-page number (OpenRead p2) in sqlite_master.
func (b *SQLiteBrowser) buildVDBESchema(instrs []vdbeInstr) *vdbeSchema {
	if b.conn == nil {
		return nil
	}
	ctx := context.Background()

	// Collect cursor→rootpage and cursor→comment-name from instructions.
	cursorRoot := make(map[int]int)    // cursor id → root page
	cursorName := make(map[int]string) // cursor id → name from comment
	for _, ins := range instrs {
		if ins.opcode == "OpenRead" || ins.opcode == "OpenWrite" {
			cursorRoot[ins.p1] = ins.p2
			if name := vdbeNameFromComment(ins.comment); name != "" {
				cursorName[ins.p1] = name
			}
		}
	}
	if len(cursorRoot) == 0 {
		return nil
	}

	// Single sqlite_master query: rootpage→name and index→underlying table.
	rootToName := make(map[int]string)
	indexToTable := make(map[string]string)
	rows, err := b.conn.Read.QueryContext(ctx,
		`SELECT rootpage, name, type, tbl_name FROM sqlite_master
		 WHERE type IN ('table', 'index')`)
	if err == nil {
		for rows.Next() {
			var rp int
			var name, typ, tblName string
			if rows.Scan(&rp, &name, &typ, &tblName) == nil {
				rootToName[rp] = name
				if typ == "index" {
					indexToTable[name] = tblName
				}
			}
		}
		rows.Close()
	}

	resolveToTable := func(objName string) string {
		if tbl, isIdx := indexToTable[objName]; isIdx {
			return tbl
		}
		return objName
	}

	schema := &vdbeSchema{
		cursorTable:  make(map[int]string),
		tableColumns: make(map[string][]string),
	}
	for cursorID, rp := range cursorRoot {
		// Strategy 1: name from EXPLAIN comment.
		if name, ok := cursorName[cursorID]; ok {
			schema.cursorTable[cursorID] = resolveToTable(name)
			continue
		}
		// Strategy 2: rootpage lookup in sqlite_master.
		if objName, ok := rootToName[rp]; ok {
			schema.cursorTable[cursorID] = resolveToTable(objName)
		}
	}

	// Load column names via PRAGMA table_info for each resolved table.
	seen := make(map[string]bool)
	for _, tbl := range schema.cursorTable {
		if seen[tbl] {
			continue
		}
		seen[tbl] = true
		prows, err := b.conn.Read.QueryContext(ctx,
			fmt.Sprintf("PRAGMA table_info(%s)", sqliteQuoteIdent(tbl)))
		if err != nil {
			continue
		}
		type colEntry struct {
			cid  int
			name string
		}
		var entries []colEntry
		maxCid := 0
		for prows.Next() {
			var cid, notNull, pk int
			var name, typ string
			var dflt sql.NullString
			if prows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk) == nil {
				entries = append(entries, colEntry{cid, name})
				if cid > maxCid {
					maxCid = cid
				}
			}
		}
		prows.Close()
		cols := make([]string, maxCid+1)
		for _, e := range entries {
			cols[e.cid] = e.name
		}
		schema.tableColumns[tbl] = cols
	}

	return schema
}

// buildSQLiteLogicalPlan derives a human-readable logical plan from VDBE
// instructions using the following deterministic algorithm:
//
//  1. Build cursor map (OpenRead / OpenWrite).
//  2. Detect all Rewind…Next loops.
//  3. Detect Bloom build blocks: a Once instruction whose body contains
//     FilterAdd. Loops that live inside a Bloom block are "build loops" and
//     are excluded from the main loop list.
//  4. For each remaining (outer) loop:
//     a. If the loop body contains a Bloom block → Nested Loop Join (Bloom
//        Optimized): "Build Bloom Filter" child + probe "Scan" child with
//        pre-bloom filters, bloom check line, rowid lookup line, post-join
//        filters, and projection.
//     b. Else if the body contains SeekRowid / NotExists on a foreign cursor
//        → plain Nested Loop Join (outer scan + inner rowid lookup).
//     c. Otherwise → single Scan / Index Scan node with filters and projection.
//  5. Outside all loops: detect Aggregate, Sort, Limit.
func buildSQLiteLogicalPlan(instrs []vdbeInstr, schema *vdbeSchema) plugin.BrowserExplainNode {

	// instructions strictly in (lo, hi) — exclusive on both ends
	between := func(lo, hi int) []vdbeInstr {
		var out []vdbeInstr
		for _, ins := range instrs {
			if ins.addr > lo && ins.addr < hi {
				out = append(out, ins)
			}
		}
		return out
	}

	// ── detect all Rewind…Next loops ──────────────────────────────────────
	type loop struct {
		cursor     int
		rewindAddr int
		nextAddr   int
	}
	var allLoops []loop
	for _, rw := range instrs {
		if rw.opcode != "Rewind" {
			continue
		}
		for _, nx := range instrs {
			if nx.opcode == "Next" && nx.p1 == rw.p1 &&
				nx.addr > rw.addr && nx.addr < rw.p2 {
				allLoops = append(allLoops, loop{rw.p1, rw.addr, nx.addr})
				break
			}
		}
	}

	// ── detect Bloom build blocks (Once + FilterAdd) ──────────────────────
	type bloomBlock struct {
		onceAddr int
		endAddr  int    // p2 of Once = first address after the block
		column   string // column name extracted from Explain opcode
	}
	var bloomBlocks []bloomBlock
	for _, ins := range instrs {
		if ins.opcode != "Once" {
			continue
		}
		onceBody := between(ins.addr, ins.p2)
		hasFilterAdd := false
		col := ""
		for _, ob := range onceBody {
			if ob.opcode == "FilterAdd" {
				hasFilterAdd = true
			}
			if ob.opcode == "Explain" && strings.Contains(ob.p4, "BLOOM FILTER") {
				col = vdbeBloomColumn(ob.p4)
			}
		}
		if hasFilterAdd {
			bloomBlocks = append(bloomBlocks, bloomBlock{ins.addr, ins.p2, col})
		}
	}

	// A loop is a "bloom build loop" when its Rewind falls inside a bloom block.
	isBloomBuildLoop := func(l loop) bool {
		for _, bb := range bloomBlocks {
			if l.rewindAddr > bb.onceAddr && l.rewindAddr < bb.endAddr {
				return true
			}
		}
		return false
	}

	var outerLoops []loop
	for _, l := range allLoops {
		if !isBloomBuildLoop(l) {
			outerLoops = append(outerLoops, l)
		}
	}

	// mark addresses inside any loop (for global-op detection)
	inAnyLoop := make(map[int]bool)
	for _, l := range allLoops {
		for _, ins := range instrs {
			if ins.addr >= l.rewindAddr && ins.addr <= l.nextAddr {
				inAnyLoop[ins.addr] = true
			}
		}
	}

	root := plugin.BrowserExplainNode{Name: "Logical Plan"}

	for _, l := range outerLoops {
		body := between(l.rewindAddr, l.nextAddr)

		// Does this outer loop contain a Bloom build block?
		var bloom *bloomBlock
		for i := range bloomBlocks {
			bb := &bloomBlocks[i]
			if bb.onceAddr > l.rewindAddr && bb.endAddr <= l.nextAddr {
				bloom = bb
				break
			}
		}

		if bloom != nil {
			// ── Nested Loop Join (Bloom Optimized) ──────────────────────────
			//
			// Body split:
			//   preBloom  = outer scan + pre-bloom filter   (rewindAddr .. onceAddr)
			//   onceBody  = bloom build inner loop           (onceAddr .. endAddr)
			//   postBloom = bloom probe + join + projection  (endAddr .. nextAddr)
			preBloom := between(l.rewindAddr, bloom.onceAddr)
			postBloom := between(bloom.endAddr-1, l.nextAddr)

			// Find the inner build loop that lives inside the bloom block.
			var buildLoop *loop
			for i := range allLoops {
				bl := &allLoops[i]
				if bl.rewindAddr > bloom.onceAddr && bl.nextAddr < bloom.endAddr {
					buildLoop = bl
					break
				}
			}

			// "Build Bloom Filter" subtree
			buildNode := plugin.BrowserExplainNode{Name: "Build Bloom Filter"}
			if buildLoop != nil {
				buildBody := between(buildLoop.rewindAddr, buildLoop.nextAddr)
				buildNode.Children = append(buildNode.Children, plugin.BrowserExplainNode{
					Name:  "Scan " + schema.tableName(buildLoop.cursor),
					Lines: vdbeFilterLines(buildBody, instrs, schema),
				})
			}

			// Probe side: outer scan node whose Lines collect all operations
			// that happen per-row in the outer loop.
			bloomCheck := "Bloom Check"
			if bloom.column != "" {
				bloomCheck = "Bloom Check " + bloom.column
			}

			probeLines := vdbeFilterLines(preBloom, instrs, schema)
			probeLines = append(probeLines,
				plugin.BrowserExplainLine{Text: bloomCheck, Highlight: true})

			joinIdx, joinCursor, joinOp := vdbeJoinPoint(postBloom, l.cursor)
			if joinIdx >= 0 {
				probeLines = append(probeLines, plugin.BrowserExplainLine{
					Text:      "Rowid Lookup " + schema.tableName(joinCursor) + " [" + joinOp + "]",
					Highlight: true,
				})
				innerBody := postBloom[joinIdx+1:]
				probeLines = append(probeLines, vdbeFilterLines(innerBody, instrs, schema)...)
				if vdbeHasResultRow(innerBody) {
					probeLines = append(probeLines,
						plugin.BrowserExplainLine{Text: "Project columns"})
				}
			} else if vdbeHasResultRow(postBloom) {
				probeLines = append(probeLines,
					plugin.BrowserExplainLine{Text: "Project columns"})
			}

			probeNode := plugin.BrowserExplainNode{
				Name:  "Scan " + schema.tableName(l.cursor),
				Lines: probeLines,
			}

			joinNode := plugin.BrowserExplainNode{Name: "Nested Loop Join (Bloom Optimized)"}
			joinNode.Children = append(joinNode.Children, buildNode, probeNode)
			root.Children = append(root.Children, joinNode)

		} else {
			// ── Plain join or single scan ────────────────────────────────────
			joinIdx, joinCursor, joinOp := vdbeJoinPoint(body, l.cursor)

			if joinIdx >= 0 {
				outerBody := body[:joinIdx]
				innerBody := body[joinIdx+1:]

				outerNode := plugin.BrowserExplainNode{
					Name:  "Scan " + schema.tableName(l.cursor),
					Lines: vdbeFilterLines(outerBody, instrs, schema),
				}
				innerNode := plugin.BrowserExplainNode{
					Name: "Rowid Lookup " + schema.tableName(joinCursor) +
						" [" + joinOp + "]",
				}
				if vdbeHasResultRow(innerBody) {
					innerNode.Lines = append(innerNode.Lines,
						plugin.BrowserExplainLine{Text: "Project result columns"})
				}

				joinNode := plugin.BrowserExplainNode{Name: "Nested Loop Join"}
				joinNode.Children = append(joinNode.Children, outerNode, innerNode)
				root.Children = append(root.Children, joinNode)

			} else {
				scanType := "Scan"
				seekOp := ""
				preBody := between(l.rewindAddr-10, l.rewindAddr)
				for _, ins := range append(preBody, body...) {
					switch ins.opcode {
					case "SeekRowid", "NotExists":
						if seekOp == "" {
							scanType = "Rowid Seek"
							seekOp = ins.opcode
						}
					case "SeekGE", "SeekGT", "SeekLE", "SeekLT":
						if scanType == "Scan" {
							scanType = "Index Scan"
							seekOp = ins.opcode
						}
					}
				}

				nodeName := scanType + " " + schema.tableName(l.cursor)
				if seekOp != "" {
					nodeName += " [" + seekOp + "]"
				}
				scanNode := plugin.BrowserExplainNode{
					Name:  nodeName,
					Lines: vdbeFilterLines(body, instrs, schema),
				}
				if vdbeHasResultRow(body) {
					scanNode.Lines = append(scanNode.Lines,
						plugin.BrowserExplainLine{Text: "Project result columns"})
				}
				root.Children = append(root.Children, scanNode)
			}
		}
	}

	// ── global operations outside all loops ───────────────────────────────
	hasAgg, hasSort, hasLimit := false, false, false
	for _, ins := range instrs {
		if inAnyLoop[ins.addr] {
			continue
		}
		switch ins.opcode {
		case "AggStep", "AggFinal":
			hasAgg = true
		case "Sort", "SorterSort", "SorterOpen":
			hasSort = true
		case "Limit":
			hasLimit = true
		}
	}
	if hasAgg {
		root.Children = append(root.Children, plugin.BrowserExplainNode{Name: "Aggregate"})
	}
	if hasSort {
		root.Children = append(root.Children, plugin.BrowserExplainNode{Name: "Sort"})
	}
	if hasLimit {
		root.Children = append(root.Children, plugin.BrowserExplainNode{Name: "Limit"})
	}

	// Fallback: scalar / constant query with no loops.
	if len(root.Children) == 0 {
		for _, ins := range instrs {
			if ins.opcode == "ResultRow" {
				root.Children = append(root.Children,
					plugin.BrowserExplainNode{Name: "Project (single row)"})
				break
			}
		}
	}

	return root
}

// vdbeBloomColumn extracts the join column from an Explain opcode's p4 text.
// Example: "BLOOM FILTER ON at (ArtistId=?)" → "ArtistId"
func vdbeBloomColumn(p4 string) string {
	start := strings.IndexByte(p4, '(')
	if start < 0 {
		return ""
	}
	rest := p4[start+1:]
	end := strings.IndexAny(rest, "=)")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// vdbeJoinPoint scans body for the first SeekRowid or NotExists instruction
// whose cursor (p1) differs from outerCursor. Returns the index within body,
// the inner cursor id, and the opcode; or (-1, -1, "") if none found.
func vdbeJoinPoint(body []vdbeInstr, outerCursor int) (idx, cursor int, op string) {
	for i, ins := range body {
		if (ins.opcode == "SeekRowid" || ins.opcode == "NotExists") && ins.p1 != outerCursor {
			return i, ins.p1, ins.opcode
		}
	}
	return -1, -1, ""
}

// vdbeTraceReg traces a register back to its assigned value.
//
// Column assignments happen at runtime (addrs < funcAddr in the instruction
// stream) while literal assignments (String8, Integer) happen in the init
// section which has higher addrs. The function checks both.
//
// Returns kind ("column", "literal", "unknown"), a literal string value if
// kind=="literal", and cursor/col indices if kind=="column".
func vdbeTraceReg(allInstrs []vdbeInstr, funcAddr, reg int) (kind, litVal string, cursor, col int) {
	// Most-recent Column assignment before funcAddr (runtime value).
	for i := len(allInstrs) - 1; i >= 0; i-- {
		ins := allInstrs[i]
		if ins.addr >= funcAddr {
			continue
		}
		if ins.opcode == "Column" && ins.p3 == reg {
			return "column", "", ins.p1, ins.p2
		}
	}
	// Global literal assignment anywhere in the program (init-section value).
	for _, ins := range allInstrs {
		switch ins.opcode {
		case "String8", "String":
			if ins.p2 == reg {
				return "literal", ins.p4, 0, 0
			}
		case "Integer":
			if ins.p2 == reg {
				return "literal", strconv.Itoa(ins.p1), 0, 0
			}
		}
	}
	return "unknown", "", 0, 0
}

// vdbeFormatFilter builds a human-readable filter description from a Function
// opcode by tracing each argument register.
//
// For "like(2)": like(pattern, value) → "table.col LIKE 'pattern'"
// For unknown functions: "funcname(arg0, arg1, ...)"
func vdbeFormatFilter(allInstrs []vdbeInstr, fn vdbeInstr, schema *vdbeSchema) string {
	name := fn.p4
	argc := 0
	if cut := strings.IndexByte(name, '('); cut > 0 {
		fmt.Sscanf(name[cut+1:], "%d", &argc)
		name = name[:cut]
	}
	if argc == 0 {
		return "Filter " + name
	}

	type arg struct {
		isLit  bool
		lit    string
		cursor int
		col    int
	}
	args := make([]arg, argc)
	for i := 0; i < argc; i++ {
		reg := fn.p2 + i
		kind, lit, cur, c := vdbeTraceReg(allInstrs, fn.addr, reg)
		switch kind {
		case "literal":
			args[i] = arg{isLit: true, lit: lit}
		case "column":
			args[i] = arg{cursor: cur, col: c}
		default:
			args[i] = arg{isLit: true, lit: fmt.Sprintf("r[%d]", reg)}
		}
	}

	desc := func(a arg) string {
		if a.isLit {
			return "'" + a.lit + "'"
		}
		if schema != nil {
			return schema.colRef(a.cursor, a.col)
		}
		return fmt.Sprintf("col[%d]", a.col)
	}

	// Well-known 2-arg infix functions.
	if argc == 2 {
		switch strings.ToLower(name) {
		case "like":
			return desc(args[1]) + " LIKE " + desc(args[0])
		case "glob":
			return desc(args[1]) + " GLOB " + desc(args[0])
		case "regexp":
			return desc(args[1]) + " REGEXP " + desc(args[0])
		}
	}

	parts := make([]string, argc)
	for i, a := range args {
		parts[i] = desc(a)
	}
	return name + "(" + strings.Join(parts, ", ") + ")"
}

// vdbeFilterLines scans a body slice and returns one BrowserExplainLine per
// detected filter. allInstrs is the full program (needed for register tracing
// across the init-section boundary); schema may be nil.
func vdbeFilterLines(body []vdbeInstr, allInstrs []vdbeInstr, schema *vdbeSchema) []plugin.BrowserExplainLine {
	var lines []plugin.BrowserExplainLine
	for i, ins := range body {
		switch ins.opcode {
		case "If", "IfNot", "IfNoHope":
			desc := "Filter"
			for j := i - 1; j >= 0 && j >= i-4; j-- {
				switch body[j].opcode {
				case "Function":
					desc = vdbeFormatFilter(allInstrs, body[j], schema)
				case "Eq", "Ne", "Lt", "Gt", "Le", "Ge", "Compare":
					desc = "Filter " + body[j].opcode
				}
				if desc != "Filter" {
					break
				}
			}
			lines = append(lines, plugin.BrowserExplainLine{Text: desc, Highlight: desc != "Filter"})
		}
	}
	return lines
}

// vdbeHasResultRow reports whether body contains a ResultRow instruction.
func vdbeHasResultRow(body []vdbeInstr) bool {
	for _, ins := range body {
		if ins.opcode == "ResultRow" {
			return true
		}
	}
	return false
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
