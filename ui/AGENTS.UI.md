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
| public                                           ◐ 500ms            |
+---------------------------------------------------------------------+
```

- **Navbar**: 3-char wide vertical icon strip, left edge
- **Sidebar**: 25% of width minus navbar, toggleable with `Ctrl+\`
  - Header: bold title line
  - Body: Tree component with 1-char padding on all sides
- **Content**: remaining width, split 50/50 vertically
  - Tabs header: tab bar — active tab teal (focused) or light gray (blurred)
  - Top half: textarea (editable, with line numbers)
  - Bottom half: results viewport
- **Footer**: single line pinned to bottom — selected item label on left, spinner + elapsed ms on right during query

## Files

| File | Struct | Purpose |
|------|--------|---------|
| `tui.go` | `TUI` | Root Bubble Tea model. Composes all components, handles input routing and layout |
| `theme.go` | `Theme` | Single source of truth for all colors. Every component receives `Theme` at construction |
| `navbar.go` | `Navbar` | Vertical icon navigation. Manages `NavItem` enum, active state, `Next()`/`Prev()` cycling |
| `sidebar.go` | `Sidebar` | Header + Tree panel |
| `tree.go` | `Tree` | Lazy-loading recursive tree with expandable/loaded state, spinner, scrolling |
| `tabs.go` | `Tabs` | Tab bar header — active tab color reflects content focus state |
| `content.go` | `Content` | Manages tabs, textareas (one per tab), and results panels. All tab switch helpers preserve focus state |
| `results.go` | `Results` | Viewport-based results panel for query output |
| `help.go` | `Help` | Centered overlay showing keyboard shortcuts, toggled with F1 |
| `footer.go` | `Footer` | Bottom status bar — context label left, spinner + ms right during queries |

## Theme

`theme.go` is the single source of truth for all colors. Components never hardcode colors — they read from the `Theme` struct passed during construction.

### Palette (ANSI 256)

| Token | Code | Hex | Usage |
|-------|------|-----|-------|
| `Bg` | 235 | `#262626` | Content background |
| `Fg` | 252 | `#d0d0d0` | Primary text |
| `Muted` | 243 | `#767676` | Secondary/placeholder text, inactive tabs |
| `Accent` | 6 | `#008080` | Interactive elements (cyan) |
| `AccentInactive` | 245 | `#8a8a8a` | Unfocused cursor in tree, active tab when content blurred |
| `Success` | 2 | `#008000` | Success status (green) |
| `Danger` | 1 | `#800000` | Error/modified indicator (red dot) |
| `Warning` | 3 | `#808000` | Warning status (yellow) |
| `FooterBg` | 30 | `#008080` | Footer bar + active tab background (teal) |
| `FooterFg` | 234 | `#1c1c1c` | Footer bar + active tab text (dark) |
| `SidebarBg` | 236 | `#303030` | Sidebar body background |
| `SidebarHeaderBg` | 237 | `#3a3a3a` | Sidebar header + tab bar filler background |
| `SidebarHeaderFg` | 252 | `#d0d0d0` | Sidebar header text |
| `NavBg` | 235 | `#262626` | Navbar background |
| `NavFg` | 116 | `#87d7d7` | Navbar icon color (light teal) |
| `NavActiveBg` | 30 | `#008080` | Active nav icon background (teal) |
| `NavActiveFg` | 234 | `#1c1c1c` | Active nav icon text (dark) |

### Design Rules

- Use ANSI 256 palette for terminal compatibility
- Never set colors directly in components — always go through `Theme`
- Active state (focused): teal (`FooterBg`/`NavActiveBg`)
- Active state (unfocused/blurred): light gray (`AccentInactive`)
- Text on teal or gray backgrounds uses dark (`FooterFg`)
- Tab bar filler uses `SidebarHeaderBg`
- Modified tabs show a red `•` dot (`Danger`) — dot background matches active tab bg

## Components

### Navbar

Icons are single unicode characters, centered in 3-char width cells with 1-line top padding per item. Supports `Next()`/`Prev()` cycling and `ActiveTitle()`.

| NavItem | Icon | Title |
|---------|------|-------|
| `NavDatabase` | `⛁` (U+26C1) | Database |
| `NavFavourites` | `★` (U+2605) | Favourites |

### Tree

Lazy-loading recursive tree. Each `TreeItem` tracks:

| Field | Type | Meaning |
|-------|------|---------|
| `ID` | string | Stable identifier used for browser path |
| `Label` | string | Display text (truncated with `…` if too wide) |
| `Children` | `[]TreeItem` | Loaded child items |
| `Expandable` | bool | Whether this item can have children (drives `▶`/`▼` icons) |
| `Expanded` | bool | Whether children are currently shown |
| `Loaded` | bool | Whether children have been fetched from DB |
| `loading` | bool (unexported) | Spinner is active while async load is in flight |

**Loading flow:**
1. User presses `D` on an unloaded expandable item
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

**Spinner frames:** `◐ ◓ ◑ ◒` (advances every 50ms tick while any item is loading)

### Tabs

`Tabs.Focused bool` controls active tab color:
- `Focused = true` (content has keyboard focus): active tab uses `FooterBg`/`FooterFg` (teal)
- `Focused = false` (tree has focus): active tab uses `AccentInactive` (light gray)

`Content.Focus()` and `Content.Blur()` set `Tabs.Focused`. All tab switching helpers (`AddTab`, `NextTab`, `PrevTab`, `GoToTab`) preserve the current focused state — switching tabs never steals focus back from the tree.

### Content

Each tab has its own `textarea.Model` (bubbles) and `Results` viewport. The textarea has line numbers enabled, no prompt prefix, and fully themed styles. New tabs are created via `AddTab()` and auto-sized based on current dimensions.

### Footer

- **Left**: selected item label when tree is focused (schema name, or table/view name — never the intermediate "Tables"/"Views" level); empty when editor is focused
- **Right**: spinner (`◐◓◑◒`) + elapsed ms while query running; final ms after done; empty otherwise

**Footer label logic:**
- Tree not focused → empty
- Cursor at depth 0 (schema) or depth 1 (Tables/Views fixed level) → shows the schema name
- Cursor at depth 2 (table or view name) → shows the table/view name

### Help

Centered overlay rendered with `lipgloss.Place`. Toggled with `F1`. Rendered over the full screen using `Bg` as whitespace background. Not included in the normal layout — `TUI.View` short-circuits to return only the help view when visible.

## Browser Plugin Interface

Defined in `ferro/plugin/browser.go`:

```go
type Browser interface {
    Connect(ctx context.Context, dsn string) error
    Disconnect(ctx context.Context) error
    List(ctx context.Context, ids []string) ([]BrowserItem, error)
    Show(ctx context.Context, ids []string) error
}

type BrowserItem struct {
    ID          string
    Name        string
    HasChildren bool
}
```

**`List` path convention (PostgreSQL):**

| `ids` | Returns |
|-------|---------|
| `[]` | All schemas from `information_schema.schemata` |
| `["schema"]` | Fixed list: `[{ID:"table", Name:"Tables"}, {ID:"view", Name:"Views"}]` |
| `["schema", "table"]` | Tables from `information_schema.tables` (BASE TABLE) |
| `["schema", "view"]` | Views from `information_schema.views` |

`Show` is called when pressing `D` on a non-expandable (leaf) item. Currently a no-op — intended for future detail/preview panels.

## Key Bindings

### Global

| Key | Action |
|-----|--------|
| `Ctrl+C` | Quit |
| `F1` | Toggle help overlay |
| `Ctrl+\` | Toggle sidebar |
| `Ctrl+T` | New tab |
| `Ctrl+R` | Run query |
| `Ctrl+W` | Toggle focus: editor ↔ tree |
| `Tab` | Next tab |
| `Shift+Tab` | Previous tab |

### Tree (when tree focused)

| Key | Action |
|-----|--------|
| `W` | Move cursor up |
| `S` | Move cursor down |
| `A` | Collapse parent (or self at root) |
| `D` | Expand/load item; if leaf, call `Show` |
| `R` (Shift+R) | Reload root list from DB |

## Options / CLI

```go
type Options struct {
    RawDriver string          // driver name (e.g. "postgresql")
    RawDSN    string          // connection string
    Registry  *plugins.Registry
}
```

Launched via `ferro ui --raw drivername:dsn`. The `--raw` value is split on the **first** `:` only. Error if no `:` present.

## Adding a Component

1. Create `ui/<name>.go` with a struct that takes `Theme` in its constructor
2. Add a `View(...)` method returning `string` — pass dimensions as arguments, use value receiver
3. Add any new color tokens to `Theme` struct and `DefaultTheme` in `theme.go`
4. Compose in `TUI` — add the field, initialize in `New()`, call `View()` in `TUI.View()`
5. Route key inputs in `TUI.Update()`
6. If resizable, update `TUI.resizeContent()` and handle in `Content.Resize()`
7. If it has a focus state, ensure `Focus()`/`Blur()` methods preserve focus when switching tabs

## Bubble Tea Gotchas

- `TUI.View()` has a **value receiver** — any mutations inside `View` are discarded. State must be updated in `Update` (which returns the new model).
- The same applies to all sub-component `View` methods — they must be value receivers.
- Scroll state (`Tree.scrollOffset`) is mutated only via `EnsureVisible()` which is a pointer receiver called from `Update`.
- `tickCmd()` schedules a 50ms recurring tick — it must be re-issued each tick as long as something is loading/running. Both the tree spinner and footer spinner share the same tick.
