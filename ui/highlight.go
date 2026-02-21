package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var sqlKeywords = map[string]bool{
	"select": true, "from": true, "where": true, "join": true,
	"left": true, "right": true, "inner": true, "outer": true,
	"full": true, "cross": true, "on": true, "and": true,
	"or": true, "not": true, "in": true, "is": true,
	"null": true, "as": true, "order": true, "by": true,
	"group": true, "having": true, "limit": true, "offset": true,
	"insert": true, "into": true, "values": true, "update": true,
	"set": true, "delete": true, "create": true, "table": true,
	"drop": true, "alter": true, "add": true, "column": true,
	"primary": true, "key": true, "foreign": true, "references": true,
	"distinct": true, "union": true, "all": true, "with": true,
	"case": true, "when": true, "then": true, "else": true,
	"end": true, "exists": true, "between": true, "like": true,
	"ilike": true, "asc": true, "desc": true, "returning": true,
	"index": true, "unique": true, "constraint": true, "default": true,
	"true": true, "false": true, "cast": true, "coalesce": true,
	"nullif": true, "over": true, "partition": true, "row": true,
	"rows": true, "range": true, "unbounded": true, "preceding": true,
	"following": true, "current": true,
}

// Matches ANSI CSI SGR sequences or identifier-like words.
var sqlTokenRe = regexp.MustCompile(`\x1b\[[0-9;]*m|[a-zA-Z_][a-zA-Z0-9_]*`)

func fgAnsiCode(color lipgloss.Color) string {
	return "\x1b[38;5;" + string(color) + "m"
}

func highlightSQL(s string, theme Theme) string {
	accentAnsi := fgAnsiCode(theme.Accent)
	restoreAnsi := fgAnsiCode(theme.Fg)

	var out strings.Builder
	last := 0

	for _, loc := range sqlTokenRe.FindAllStringIndex(s, -1) {
		out.WriteString(s[last:loc[0]])
		tok := s[loc[0]:loc[1]]

		if tok[0] == '\x1b' {
			out.WriteString(tok)
		} else if sqlKeywords[strings.ToLower(tok)] {
			out.WriteString(accentAnsi)
			out.WriteString(tok)
			out.WriteString(restoreAnsi)
		} else {
			out.WriteString(tok)
		}

		last = loc[1]
	}

	out.WriteString(s[last:])
	return out.String()
}
