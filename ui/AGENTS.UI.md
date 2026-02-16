# AGENTS.UI.md

## Overview

The `ui/` package implements a fullscreen terminal UI (TUI) for ferroDB using [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss). It is launched via `ferro ui`.

## Layout

```
+---+------------------+----------------------------------------------+
| N |     Sidebar      |                  Content                     |
| a |     (Tree)       |                                              |
| v |                  |                                              |
| b |                  |                                              |
| a |                  |                                              |
| r |                  |                                              |
+---+------------------+----------------------------------------------+
| Footer                                                              |
+---------------------------------------------------------------------+
```

- **Navbar**: 3-char wide vertical icon strip, left edge
- **Sidebar**: 25% of width minus navbar, tree/list area
- **Content**: remaining 75% of width
- **Footer**: single line pinned to bottom, vim-style status bar

## Files

| File | Struct | Purpose |
|------|--------|---------|
| `tui.go` | `TUI` | Root Bubble Tea model. Composes all components, handles input routing and layout |
| `theme.go` | `Theme` | Single source of truth for all colors. Every component receives `Theme` at construction |
| `navbar.go` | `Navbar` | Vertical icon navigation. Manages `NavItem` enum and active state |
| `sidebar.go` | `Sidebar` | Tree/list panel next to navbar |
| `content.go` | `Content` | Main content area |
| `footer.go` | `Footer` | Bottom status bar with left/right aligned text |

## Theme

`theme.go` is the single source of truth for all colors. Components never hardcode colors — they read from the `Theme` struct passed during construction.

### Palette (ANSI 256)

| Token | Code | Usage |
|-------|------|-------|
| `Bg` | 235 | Content background, navbar background |
| `Fg` | 252 | Primary text |
| `Muted` | 243 | Secondary/placeholder text |
| `Accent` | 6 | Interactive elements (cyan) |
| `Success` | 2 | Success status (green) |
| `Danger` | 1 | Error status (red) |
| `Warning` | 3 | Warning status (yellow) |
| `FooterBg` | 134 | Footer bar background (purple) |
| `FooterFg` | 234 | Footer bar text (dark) |
| `SidebarBg` | 236 | Sidebar background (slightly lighter than content) |
| `NavBg` | 235 | Navbar background (matches content) |
| `NavFg` | 183 | Navbar icon color (light purple) |
| `NavActiveBg` | 134 | Active nav icon background (purple) |
| `NavActiveFg` | 234 | Active nav icon text (dark) |

### Design Rules

- Use ANSI 256 palette for terminal compatibility
- Never set colors directly in components — always go through `Theme`
- Reverse video for the footer (bold + colored background)
- Active nav items get a purple highlight background with dark icon; inactive items show light purple icon on content background
- Padding/spacing never inherits active highlight colors

## Navbar

Icons are single unicode characters, centered in 3-char width cells with 1-line top padding between items. The padding always uses `NavBg`, never the active highlight color.

| Key | NavItem | Icon |
|-----|---------|------|
| `1` | `NavDatabase` | `U+26C1` White Draughts King |
| `2` | `NavFavourites` | `U+2605` Black Star |

## Adding a Component

1. Create `ui/<name>.go` with a struct that takes `Theme` in its constructor
2. Add a `View(...)` method that returns a `string` — pass dimensions as arguments
3. Add any new color tokens to `Theme` struct and `DefaultTheme` in `theme.go`
4. Compose it in `TUI` — add the field, initialize in `New()`, call `View()` in `TUI.View()`
5. Route any key inputs in `TUI.Update()`

## Key Bindings

| Key | Action |
|-----|--------|
| `q` / `ctrl+c` | Quit |
| `1` | Switch to Database nav |
| `2` | Switch to Favourites nav |
