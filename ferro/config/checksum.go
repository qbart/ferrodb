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
	raw = CleanTrimLineEnds(raw)
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

func CleanTrimLineEnds(raw []byte) []byte {
	out := make([]byte, 0, len(raw))

	for _, b := range raw {
		if b == '\n' {
			for len(out) > 0 {
				last := out[len(out)-1]
				if last != ' ' && last != '\t' {
					break
				}
				out = out[:len(out)-1]
			}
			out = append(out, b)
			continue
		}
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
