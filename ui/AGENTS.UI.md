# AGENTS.UI.md

## Overview

The `ui/` package implements a fullscreen terminal UI (TUI) for ferroDB using [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss). It is launched via `ferro ui`.

## Layout

```
+---+------------------+----------------------------------------------+
| N |  Sidebar Header  | [Tabs: 1 | 2 | 3]                           |
| a |  (Tree)          |----------------------------------------------|
| v |   public         | SELECT u.id, u.name                         |
| b |     users        | FROM users u                                 |
| a |     orders       | WHERE ...                     (textarea)     |
| r |     products     |----------------------------------------------|
|   |   auth           | id | name    | email          (results)      |
|   |   analytics      |  1 | Alice   | alice@...                     |
+---+------------------+----------------------------------------------+
| ferroDB                                          ◐ 500ms            |
+---------------------------------------------------------------------+
```

- **Navbar**: 3-char wide vertical icon strip, left edge
- **Sidebar**: 25% of width minus navbar, toggleable with `Ctrl+\`
  - Header: bold title line (changes with nav selection)
  - Body: Tree component with 1-char padding on all sides
- **Content**: remaining width, split 50/50 vertically
  - Tabs header: tab bar with active tab highlighted in teal
  - Top half: textarea (editable, with line numbers)
  - Bottom half: results viewport
- **Footer**: single line pinned to bottom, spinner + elapsed ms during query

## Files

| File | Struct | Purpose |
|------|--------|---------|
| `tui.go` | `TUI` | Root Bubble Tea model. Composes all components, handles input routing and layout |
| `theme.go` | `Theme` | Single source of truth for all colors. Every component receives `Theme` at construction |
| `navbar.go` | `Navbar` | Vertical icon navigation. Manages `NavItem` enum, active state, `Next()`/`Prev()` cycling |
| `sidebar.go` | `Sidebar` | Header + Tree panel, toggleable visibility |
| `tree.go` | `Tree` | Recursive tree view with `TreeItem` model (label, children, expanded) |
| `tabs.go` | `Tabs` | Tab bar header with `TabItem` model (title, content, modified flag with red dot) |
| `content.go` | `Content` | Manages tabs, textareas (one per tab), and results panels. Handles 50/50 split |
| `results.go` | `Results` | Viewport-based results panel for query output |
| `help.go` | `Help` | Centered overlay showing keyboard shortcuts, toggled with F1 |
| `footer.go` | `Footer` | Bottom status bar with spinner animation during queries |

## Theme

`theme.go` is the single source of truth for all colors. Components never hardcode colors — they read from the `Theme` struct passed during construction.

### Palette (ANSI 256)

| Token | Code | Usage |
|-------|------|-------|
| `Bg` | 235 | Content background |
| `Fg` | 252 | Primary text |
| `Muted` | 243 | Secondary/placeholder text, inactive tabs |
| `Accent` | 6 | Interactive elements (cyan) |
| `Success` | 2 | Success status (green) |
| `Danger` | 1 | Error/modified indicator (red dot) |
| `Warning` | 3 | Warning status (yellow) |
| `FooterBg` | 30 | Footer bar background (teal) |
| `FooterFg` | 234 | Footer bar text (dark) |
| `SidebarBg` | 236 | Sidebar body background |
| `SidebarHeaderBg` | 237 | Sidebar header + tab bar filler background |
| `SidebarHeaderFg` | 252 | Sidebar header text |
| `NavBg` | 235 | Navbar background |
| `NavFg` | 116 | Navbar icon color (light teal) |
| `NavActiveBg` | 30 | Active nav icon background (teal) |
| `NavActiveFg` | 234 | Active nav icon text (dark) |

### Design Rules

- Use ANSI 256 palette for terminal compatibility
- Never set colors directly in components — always go through `Theme`
- Accent color is teal (30) used for footer, active nav, and active tabs
- Text on teal backgrounds is dark (234)
- Active tabs use teal bg; inactive tabs use content bg
- Tab bar filler uses sidebar header bg
- Modified tabs show a red `•` dot (Danger color)

## Components

### Navbar

Icons are single unicode characters, centered in 3-char width cells with 1-line top padding. Supports `Next()`/`Prev()` cycling and `ActiveTitle()`.

| NavItem | Icon | Title |
|---------|------|-------|
| `NavDatabase` | `U+26C1` | Database |
| `NavFavourites` | `U+2605` | Favourites |

### Tree

Recursive tree with `TreeItem` model. Each item has `Label`, `Children []TreeItem`, and `Expanded` bool. Renders with 2-space indentation, `▶`/`▼` prefixes for nodes with children.

### Tabs

`TabItem` has `Title`, `Content`, and `Modified` bool. Active tab renders with teal footer colors. Modified tabs show a red `•` indicator.

### Content

Each tab has its own `textarea.Model` (bubbles) and `Results` viewport. The textarea has line numbers enabled, no prompt prefix, and themed styles. New tabs are created via `AddTab()` and auto-sized.

### Footer

Shows `ferroDB` on the left. Right side shows spinner (`◐◓◑◒`) + elapsed ms during query execution, final ms after completion, empty otherwise. Spinner advances on 50ms tick.

## Key Bindings

| Key | Action |
|-----|--------|
| `Ctrl+C` | Quit |
| `F1` | Toggle help overlay |
| `Ctrl+\` | Toggle sidebar |
| `Ctrl+T` | New tab |
| `Ctrl+R` | Run query (fake 500ms) |
| `Tab` | Next tab |
| `Shift+Tab` | Previous tab |

## Adding a Component

1. Create `ui/<name>.go` with a struct that takes `Theme` in its constructor
2. Add a `View(...)` method that returns a `string` — pass dimensions as arguments
3. Add any new color tokens to `Theme` struct and `DefaultTheme` in `theme.go`
4. Compose it in `TUI` — add the field, initialize in `New()`, call `View()` in `TUI.View()`
5. Route any key inputs in `TUI.Update()`
6. If the component needs resizing, update `TUI.resizeContent()` and handle in `AddTab()`
