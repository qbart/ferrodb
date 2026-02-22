# AGENTS.UI.md

## Overview

The `ui/` package implements a fullscreen terminal UI (TUI) for ferroDB using [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss). It is launched via `ferro ui --raw drivername:dsn`.

## Layout

```
+---+------------------+----------------------------------------------+
| N |  Sidebar Header  | [1] [2] [3]                                  |
| a |  (Tree)          |----------------------------------------------|
| v |   ▼ public       | SELECT u.id, u.name                         |
| b |     ▶ Tables     | FROM users u                                 |
| a |     ▶ Views      | WHERE ...                     (textarea)     |
| r |   ▶ auth         |----------------------------------------------|
|   |   ▶ analytics    | id | name    | email          (results)      |
|   |                  |  1 | Alice   | alice@...                     |
+---+------------------+----------------------------------------------+
| ↑ ↓  j k  scroll    ctrl+g  switch view              ◐ 500ms       |
+---------------------------------------------------------------------+
```

- **Navbar**: 3-char wide vertical icon strip, left edge
- **Sidebar**: 25% of width minus navbar, toggleable with `Ctrl+\`
  - Header: bold title line
  - Body: Tree component with 1-char padding on all sides
- **Content**: remaining width, split 50/50 vertically
  - Tabs header: tab bar — active tab accent (focused) or light gray (blurred)
  - Top half: textarea (editable, with line numbers)
  - Bottom half: results viewport
- **Footer**: single line pinned to bottom — context label/shortcuts on left, spinner + elapsed ms on right during query

## Files

| File | Struct | Purpose |
|------|--------|---------|
| `tui.go` | `TUI` | Root Bubble Tea model. Composes all components, handles input routing and layout |
| `theme.go` | `Theme` | Single source of truth for all colors. Package-level `Accent`, `AccentInactive`, `Black` vars for direct use |
| `navbar.go` | `Navbar` | Vertical icon navigation. Manages `NavItem` enum, active state, `Next()`/`Prev()` cycling |
| `sidebar.go` | `Sidebar` | Header + Tree panel |
| `tree.go` | `Tree` | Lazy-loading recursive tree with expandable/loaded state, spinner, scrolling |
| `tabs.go` | `Tabs` | Tab bar header — active tab color reflects content focus state |
| `content.go` | `Content` | Manages tabs, textareas (one per tab), and results panels. All tab switch helpers preserve focus state |
| `results.go` | `Results` | Viewport-based results panel for query output |
| `highlight.go` | — | SQL keyword highlighting — post-processes textarea ANSI output |
| `help.go` | `Help` | Centered overlay showing keyboard shortcuts, toggled with F1 |
| `row_detail.go` | `RowDetail` | Fullscreen overlay showing all column values for selected result row |
| `explain_view.go` | `ExplainView` | Scrollable explain plan view — hierarchical node boxes + per-node-type stats table |
| `footer.go` | `Footer` | Bottom status bar — context label/shortcuts left, spinner + ms right during queries |
| `clipboard.go` | — | `ClipboardWrite([]byte)` / `ClipboardWriteString(string)` helpers using `go-nativeclipboard` |

## Theme

`theme.go` is the single source of truth for all colors. Components never hardcode colors — they read from the `Theme` struct passed during construction, or use the package-level color variables directly.

### Package-level color variables

```go
var (
    Accent         = lipgloss.Color("37")
    AccentInactive = lipgloss.Color("245")
    Black          = lipgloss.Color("234")
)
```

Use these directly in helper functions that do not receive a `Theme` (e.g. `buildNodeBox`, `buildExplainTableLines`).

### Palette (ANSI 256)

| Token | Code | Usage |
|-------|------|-------|
| `Bg` | 235 | Content background |
| `Fg` | 252 | Primary text |
| `Muted` | 243 | Secondary/placeholder text, inactive tabs |
| `Accent` | 37 | Interactive elements, node names, table titles |
| `AccentInactive` | 245 | Unfocused cursor in tree, active tab when content blurred |
| `Success` | 2 | Success status (green) |
| `Danger` | 1 | Error/modified indicator (red dot) |
| `Warning` | 3 | Warning status (yellow) |
| `FooterBg` | 37 (=Accent) | Footer bar + active tab background |
| `FooterFg` | 234 (=Black) | Footer bar + active tab text (dark) |
| `SidebarBg` | 236 | Sidebar body background |
| `SidebarHeaderBg` | 237 | Sidebar header + tab bar filler background |
| `SidebarHeaderFg` | 252 | Sidebar header text |
| `NavBg` | 235 | Navbar background |
| `NavFg` | 37 (=Accent) | Navbar icon color |
| `NavActiveBg` | 37 (=Accent) | Active nav icon background |
| `NavActiveFg` | 234 (=Black) | Active nav icon text (dark) |

### Design Rules

- Use ANSI 256 palette for terminal compatibility
- Never set colors directly in components — always go through `Theme` or the package-level vars
- **Every `lipgloss.NewStyle()` call must explicitly set `Bold(true)` or `Bold(false)`** — bold bleeds through concatenated renders if left unset
- Active state (focused): accent (`FooterBg`/`NavActiveBg`)
- Active state (unfocused/blurred): light gray (`AccentInactive`)
- Text on accent or gray backgrounds uses dark (`FooterFg`/`Black`)
- Tab bar filler uses `SidebarHeaderBg`
- Modified tabs show a red `•` dot (`Danger`) — dot background matches active tab bg

## Components

### Navbar

Single unicode icon, centered in 3-char wide cell with 1-line top padding per item. Supports `Next()`/`Prev()` cycling and `ActiveTitle()`.

| NavItem | Icon | Title |
|---------|------|-------|
| `NavDatabase` | `■` U+25A0 filled square | Data |
| `NavExplain` | `●` U+25CF filled circle | Explain |

### Tree

Lazy-loading recursive tree. Each `TreeItem` tracks:

| Field | Type | Meaning |
|-------|------|---------|
| `ID` | string | Stable identifier used for browser path |
| `Label` | string | Display text (truncated with `…` if too wide) |
| `Children` | `[]TreeItem` | Loaded child items |
| `Expandable` | bool | Whether this item can have children (drives `›`/`⌄` icons) |
| `Expanded` | bool | Whether children are currently shown |
| `Loaded` | bool | Whether children have been fetched from DB |
| `loading` | bool (unexported) | Spinner is active while async load is in flight |

**Loading flow:**
1. User presses `→` on an unloaded expandable item
2. `StartLoading()` sets `loading = true`, returns the full `[]string` ID path (root → cursor)
3. `loadItemCmd` dispatches async `browser.List(ctx, ids)` + starts tick for spinner
4. On `itemLoadedMsg`: `SetLoaded(ids, children)` walks the ID path, sets `loading = false`, `Loaded = true`, `Expanded = true`, and stores children
5. `EnsureVisible(height)` is called to keep cursor in view

**Scroll:** `scrollOffset int` is managed exclusively via `EnsureVisible(height int)`, which is called from `TUI.Update` after every navigation action. `View` is a value receiver — it only reads `scrollOffset`, never mutates it.

**Collapse behaviour:**
- Always tries to collapse the **parent** and move cursor to it
- At root level (no parent): collapses the item itself if expandable

**Expand behaviour:**
- Only expands if `Expandable && Loaded`; otherwise `StartLoading()` handles it first

**Key helpers:**

| Method | Returns | Purpose |
|--------|---------|---------|
| `StartLoading()` | `([]string, bool)` | Sets loading, returns full ID path |
| `SetLoaded(ids, children)` | — | Finalises async load at path |
| `EnsureVisible(height)` | — | Adjusts `scrollOffset` to keep cursor visible |
| `CursorExpandable()` | bool | Whether cursor item is expandable |
| `CursorIDPath()` | `[]string` | Full ID path to cursor (for Show/Load) |
| `CursorPath()` | `([]string, bool)` | Full label path to cursor (for footer display) |
| `CursorLabel()` | string | Label of the cursor item (for clipboard copy) |

**Spinner frames:** `◐ ◓ ◑ ◒` (advances every 50ms tick while any item is loading)

### Tabs

`Tabs.Focused bool` controls active tab color:
- `Focused = true` (content has keyboard focus): active tab uses `FooterBg`/`FooterFg` (accent)
- `Focused = false` (tree has focus): active tab uses `AccentInactive` (light gray)

`Content.Focus()` and `Content.Blur()` set `Tabs.Focused`. All tab switching helpers (`AddTab`, `NextTab`, `PrevTab`, `GoToTab`) preserve the current focused state — switching tabs never steals focus back from the tree.

### Content

Each tab has its own `textarea.Model` (bubbles) and `Results` viewport. The textarea has line numbers enabled, no prompt prefix, and fully themed styles. New tabs are created via `AddTab()` and auto-sized based on current dimensions.

### ExplainView

Activated via `Ctrl+E` (parses active tab's result data as JSON EXPLAIN ANALYZE output) or `Ctrl+G` (switches to the NavExplain nav section).

Renders two sections:
1. **Plan tree** — each node is a Unicode box (`┌─┐│└─┘`) with `├─▶`/`└─▶` connectors to children. Node name in bold accent. Content lines in muted color, highlighted lines in yellow.
2. **Summary lines** — planning/execution time, sort/hash memory totals (plugin-owned, `SummaryLines []BrowserExplainLine`)
3. **Tables** — e.g. "Per node type stats" with title in bold accent, separator lines, column-aligned rows

Box sizing: each depth level = 4 terminal cells. `boxContentWidth = totalWidth - nodeDepth*4 - 4`.

Bordered cell rendering splits into three segments to avoid bold bleed:
```
borderStyle.Render(indent+"│ ") + contentStyle.Render(text) + borderStyle.Render(" │")
```
`contentStyle.Bold(line.bold)` is always explicit.

Scrolling: `rowOffset int`, `ScrollUp()` / `ScrollDown(height, width int)`. `buildLines(width)` is called both for rendering and for computing scroll bounds.

### Footer

- **Explain mode**: shows `↑ ↓  j k  scroll    ctrl+g  switch view`
- **Tree focused**: shows selected item label (schema name, or table/view name — never the intermediate "Tables"/"Views" level)
- **Otherwise**: empty left side
- **Right**: spinner (`◐◓◑◒`) + elapsed ms while query running; final ms after done; empty otherwise

**Footer label logic (tree focused):**
- Depth 0 (schema) or depth 1 (Tables/Views/… fixed level) → schema name
- Depth 2 (table or view name) → table/view name
- Depth 3 (sub-category: Columns/Indexes/…) → table name
- Depth 4 (column/index/constraint name) → item name

### Help

Centered overlay rendered with `lipgloss.Place`. Toggled with `F1` (or closed with `Esc`/`Ctrl+C`). Rendered over the full screen using `Bg` as whitespace background. Intercepts all keys — `TUI.Update` checks `help.Visible` before the main key switch and returns early. `TUI.View` short-circuits to return only the help view when visible.

## Browser Plugin Interface

Defined in `ferro/plugin/browser.go`:

```go
type Browser interface {
    Connect(ctx context.Context, dsn string) error
    Disconnect(ctx context.Context) error
    List(ctx context.Context, ids []string) ([]BrowserItem, error)
    Show(ctx context.Context, ids []string) (string, error)
    Query(ctx context.Context, sql string) (BrowserQueryResult, error)
    ParseExplain(data BrowserQueryResult) (BrowserExplainResult, error)
}

type BrowserItem struct {
    ID          string  // stable identifier for path operations
    Name        string  // display label (may differ, e.g. "col_name (data_type)")
    HasChildren bool
}

type BrowserExplainLine struct {
    Text      string
    Highlight bool
}

type BrowserExplainNode struct {
    Name     string
    Lines    []BrowserExplainLine
    Children []BrowserExplainNode
}

type BrowserExplainRow struct {
    Cells     []string
    Highlight bool
}

type BrowserExplainTable struct {
    Title   string
    Headers []string
    Rows    []BrowserExplainRow
}

type BrowserExplainResult struct {
    Root         BrowserExplainNode
    SummaryLines []BrowserExplainLine  // plugin-owned footer (timing, memory)
    Tables       []BrowserExplainTable // plugin-owned analytics tables
}
```

**`ParseExplain`**: receives the raw `BrowserQueryResult` (expected to be JSON EXPLAIN ANALYZE output), parses it, and builds the full `BrowserExplainResult` hierarchy. The view is a pure consumer — all business logic lives in the plugin.

**`List` path convention (PostgreSQL):**

| `ids` | Returns |
|-------|---------|
| `[]` | All schemas from `information_schema.schemata` |
| `["schema"]` | 10 object type categories: Tables, Views, Materialized Views, Functions, Types, Enums, Domains, Composite Types, Sequences, Foreign Tables |
| `["schema", "table"]` | Tables (`HasChildren: true`) |
| `["schema", "view"]` | Views |
| `["schema", "matview"]` | Materialized views |
| `["schema", "function"]` | Functions from `information_schema.routines` |
| `["schema", "type"]` | Range types (`typtype='r'`) |
| `["schema", "enum"]` | Enum types (`typtype='e'`) |
| `["schema", "domain"]` | Domain types (`typtype='d'`) |
| `["schema", "composite"]` | Composite types (`typtype='c'`, `relkind='c'`) |
| `["schema", "sequence"]` | Sequences |
| `["schema", "foreign_table"]` | Foreign tables |
| `["schema", "table", "name"]` | Sub-categories: Columns, Indexes, Constraints, Triggers, Policies, Partitions (if partitioned) |
| `["schema", "table", "name", "column"]` | `"col_name (data_type)"` labels, `ID = col_name` |
| `["schema", "table", "name", "index"]` | Index names |
| `["schema", "table", "name", "constraint"]` | Constraint names |
| `["schema", "table", "name", "trigger"]` | Trigger names |
| `["schema", "table", "name", "partition"]` | Partition names |
| `["schema", "table", "name", "policy"]` | Policy names |

**`Show` path convention (PostgreSQL):**

| `ids` | Returns |
|-------|---------|
| `["schema", "table"/"view"/"matview"/"foreign_table", "name"]` | `SELECT * FROM schema.name LIMIT 100` |
| `["schema", "enum", "name"]` | Query returning all enum values ordered by sort position |
| `["schema", "table", "name", "index", "index_name"]` | Detailed index info query (size, stats, definition) |
| anything else | `""` (no-op) |

`Show` is called when pressing `Enter` on a tree item. Returns a SQL query string — `TUI` opens a new tab, pastes the query, and runs it immediately. Returns `""` to silently do nothing.

## Key Bindings

### Global

| Key | Action |
|-----|--------|
| `Ctrl+C` | Quit |
| `F1` | Toggle help overlay |
| `Tab` | Cycle focus forward: tree → editor → results → tree (skips results if empty, skips tree if hidden) |
| `Shift+Tab` | Cycle focus backward |
| `Ctrl+\` | Toggle sidebar (auto-moves focus to editor if tree was focused) |
| `Ctrl+G` | Cycle nav (Data ↔ Explain) |
| `Ctrl+T` | New tab |
| `Ctrl+W` | Close active tab (only when editor focused; no-op if only one tab) |
| `Ctrl+R` | Run query (uses active tab's textarea content) |
| `Ctrl+E` | Parse active result as EXPLAIN ANALYZE, switch to Explain view |
| `Ctrl+Y` | Copy active tab's query text to clipboard (editor focus) |
| `Ctrl+Left` / `Ctrl+H` | Previous tab |
| `Ctrl+Right` / `Ctrl+L` | Next tab |

### Explain view (when NavExplain active)

| Key | Action |
|-----|--------|
| `↑` / `k` | Scroll up |
| `↓` / `j` | Scroll down |

### Tree (when tree focused)

| Key | Action |
|-----|--------|
| `↑` / `k` | Move cursor up |
| `↓` / `j` | Move cursor down |
| `←` / `h` | Collapse parent (or self at root level) |
| `→` / `l` | Expand / load item |
| `Enter` | Open item query in new tab and run it immediately |
| `Shift+R` | Reload root list from DB |
| `y` | Copy cursor item label to clipboard |

### Results (when results focused)

| Key | Action |
|-----|--------|
| `↑` / `k` | Move row cursor up |
| `↓` / `j` | Move row cursor down |
| `←` / `h` | Scroll columns left (or chars left in single-column mode) |
| `→` / `l` | Scroll columns right (or chars right in single-column mode) |
| `Enter` | Open fullscreen row detail overlay |

### Row Detail overlay

| Key | Action |
|-----|--------|
| `↑` | Scroll up one line |
| `↓` | Scroll down one line |
| `j` | Jump to next column header |
| `k` | Jump to previous column header |
| `←` | Scroll content left (character offset) |
| `→` | Scroll content right (character offset) |
| `y` | Copy selected column value to clipboard |
| `Enter` / `Esc` / `Ctrl+C` | Close overlay |

## Clipboard

All clipboard writes go through `ui/clipboard.go`:

```go
func ClipboardWriteString(data string)
func ClipboardWrite(data []byte)
```

Uses `github.com/aymanbagabas/go-nativeclipboard`. No initialization call needed — errors are silently ignored. Used in: `row_detail.go` (`y` copies column value), `tui.go` (`Ctrl+Y` copies editor text, `y` in tree copies item label).

## Options / CLI

```go
type Options struct {
    RawDriver string          // driver name (e.g. "postgresql")
    RawDSN    string          // connection string
    Registry  *plugins.Registry
    Version   string
}
```

Launched via `ferro ui --raw drivername:dsn`. The `--raw` value is split on the **first** `:` only. Error if no `:` present.

## SQL Keyword Highlighting

`highlight.go` post-processes the textarea's rendered output to colorize SQL keywords with `theme.Accent`. It works by tokenizing the raw ANSI-escaped string:

1. Match tokens with `\x1b\[[0-9;]*m|[a-zA-Z_][a-zA-Z0-9_]*`
2. ANSI CSI sequences pass through unchanged
3. Words matching the SQL keyword set (case-insensitive) are wrapped with `\x1b[38;5;{Accent}m` … `\x1b[38;5;{Fg}m`
4. Only foreground is changed — backgrounds (cursor line, sidebar) remain correct

Keywords covered: `SELECT FROM WHERE JOIN LEFT RIGHT INNER OUTER FULL CROSS ON AND OR NOT IN IS NULL AS ORDER BY GROUP HAVING LIMIT OFFSET INSERT INTO VALUES UPDATE SET DELETE CREATE TABLE DROP ALTER ADD COLUMN PRIMARY KEY FOREIGN REFERENCES DISTINCT UNION ALL WITH CASE WHEN THEN ELSE END EXISTS BETWEEN LIKE ILIKE ASC DESC RETURNING INDEX UNIQUE CONSTRAINT DEFAULT TRUE FALSE CAST COALESCE NULLIF OVER PARTITION ROW ROWS RANGE UNBOUNDED PRECEDING FOLLOWING CURRENT`

**Note**: `fgAnsiCode(color lipgloss.Color)` only works with numeric ANSI 256-color strings (which the default theme uses). If hex colors are ever added to the theme, this function must be updated.

## Results Table

`results.go` renders a scrollable table from `ResultData{Headers, Rows, ColumnTypes}`.

- **Header row**: always visible, 1 line — `SidebarHeaderBg`/`Fg` when focused, `SidebarBg`/`Muted` when not
- **Row cursor**: accent (`NavActiveBg`/`NavActiveFg`) when focused, light gray (`AccentInactive`/`FooterFg`) when not
- **Vertical scroll**: `cursor int` + `rowOffset int`; `EnsureVisible(height)` called from `Update` after movement
- **Horizontal scroll**: `colOffset int` shifts which column is leftmost (multi-column); `charOffset int` scrolls character-by-character within the cell (single-column)
- **Column widths**: computed once on `SetData` via `computeWidths`; capped at `maxColWidth = 50` (multi-column) or `maxColWidthSingle = 200` (single-column)
- **Cell normalization**: `normalizeCell(s, maxWidth)` strips everything after the first `\n` and truncates with `…`
- **Empty state**: styled blank block when no headers

### Row Detail overlay (`row_detail.go`)

Fullscreen overlay triggered by `Enter` on a focused result row. Shows all column→value pairs as full-width lines:
- **Column header line**: `SidebarBg` + `Accent`, bold; selected column uses `NavActiveBg`/`NavActiveFg`
- **Value lines**: `Bg` + `Fg`, full width
- **Hint bar**: last line, `FooterBg` + black text
- **`colCursor int`**: tracks selected column; updates automatically during line scroll (`syncColCursor`)
- **`rowOffset int`**: line-based vertical scroll; auto-set to selected column's header line on `JumpNext`/`JumpPrev`
- **`colOffset int`**: character-level horizontal scroll (rune offset applied via `cropRunes`)
- **Overlay intercepts all keys** before the main switch — `Ctrl+C` closes it instead of quitting

### Column Types

`ResultData.ColumnTypes []string` carries one of: `string`, `float64`, `int64`, `uint64`, `object`. Set by the browser plugin per column. The UI layer stores it for downstream use (e.g., per-type cell formatting).

## Adding a Component

1. Create `ui/<name>.go` with a struct that takes `Theme` in its constructor
2. Add a `View(...)` method returning `string` — pass dimensions as arguments, use value receiver
3. Every `lipgloss.NewStyle()` must include `.Bold(true)` or `.Bold(false)` explicitly
4. Add any new color tokens to `Theme` struct and `DefaultTheme` in `theme.go`
5. Compose in `TUI` — add the field, initialize in `New()`, call `View()` in `TUI.View()`
6. Route key inputs in `TUI.Update()`
7. If resizable, update `TUI.resizeContent()` and handle in `Content.Resize()`
8. If it has a focus state, ensure `Focus()`/`Blur()` methods preserve focus when switching tabs

## Bubble Tea Gotchas

- `TUI.View()` has a **value receiver** — any mutations inside `View` are discarded. State must be updated in `Update` (which returns the new model).
- The same applies to all sub-component `View` methods — they must be value receivers.
- Scroll state (`Tree.scrollOffset`) is mutated only via `EnsureVisible()` which is a pointer receiver called from `Update`.
- `tickCmd()` schedules a 50ms recurring tick — it must be re-issued each tick as long as something is loading/running. Both the tree spinner and footer spinner share the same tick.
- **Bold bleed**: when concatenating `Render()` calls with `+`, bold state from one segment can carry into the next if the following style does not explicitly set `Bold(false)`. Always set Bold explicitly on every style.
