package config

import (
	"crypto/sha256"
	"fmt"
)

type Checksum string

func CalculateChecksum(raw []byte) Checksum {
    raw = CleanForChecksum(raw)
	sum := sha256.Sum256(raw)
	return Checksum(fmt.Sprintf("%x", sum[:]))
}

func CleanForChecksum(raw []byte) []byte {
	raw = CleanNormalizeNewlines(raw)
	raw = CleanStripComments(raw)
	raw = CleanCollapseWhitespace(raw)
	raw = CleanTrim(raw)
	return raw
}

func CleanNormalizeNewlines(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	for _, b := range raw {
		if b == '\r' {
			continue
		}
		out = append(out, b)
	}
	return out
}

func CleanStripComments(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	inQuotes := false

	for i := 0; i < len(raw); i++ {
		c := raw[i]

		switch c {
		case '"':
			inQuotes = !inQuotes
			out = append(out, c)

		case '#':
			if !inQuotes {
				// skip until newline
				for i < len(raw) && raw[i] != '\n' {
					i++
				}
				out = append(out, '\n')
			} else {
				out = append(out, c)
			}

		default:
			out = append(out, c)
		}
	}
	return out
}

func CleanCollapseWhitespace(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	space := false

	for _, b := range raw {
		if b == ' ' || b == '\t' {
			if !space {
				out = append(out, ' ')
			}
			space = true
			continue
		}
		space = false
		out = append(out, b)
	}
	return out
}

func CleanTrim(raw []byte) []byte {
	start := 0
	end := len(raw)

	for start < end && (raw[start] == ' ' || raw[start] == '\n' || raw[start] == '\t') {
		start++
	}
	for end > start && (raw[end-1] == ' ' || raw[end-1] == '\n' || raw[end-1] == '\t') {
		end--
	}
	return raw[start:end]
}
