package config

import "testing"

func TestCleanNormalizeNewlines(t *testing.T) {
	raw := []byte("line1\r\nline2\rline3\nline4")
	expected := []byte("line1\nline2line3\nline4")

	if got := CleanNormalizeNewlines(raw); string(got) != string(expected) {
		t.Fatalf("CleanNormalizeNewlines = %q, want %q", got, expected)
	}
}

func TestCleanTrimLineEnds(t *testing.T) {
	raw := []byte("line 1   \nline 2\t\nline 3")
	expected := []byte("line 1\nline 2\nline 3")

	if got := CleanTrimLineEnds(raw); string(got) != string(expected) {
		t.Fatalf("CleanTrimLineEnds = %q, want %q", got, expected)
	}
}

func TestCleanTrim(t *testing.T) {
	raw := []byte("\n \tvalue with padding\t \n")
	expected := []byte("value with padding")

	if got := CleanTrim(raw); string(got) != string(expected) {
		t.Fatalf("CleanTrim = %q, want %q", got, expected)
	}
}

func TestCleanForChecksumPipeline(t *testing.T) {
	raw := []byte("\r\n  value \t\t with \r\n whitespace \n ")
	expected := []byte("value \t\t with\n whitespace")

	if got := CleanForChecksum(raw); string(got) != string(expected) {
		t.Fatalf("CleanForChecksum = %q, want %q", got, expected)
	}
}

func TestCalculateChecksumIgnoresTrailingWhitespace(t *testing.T) {
	a := []byte("spec:\n  field: 1\n")
	b := []byte("spec:\n  field: 1  \n\n")

	if CalculateChecksum(a) != CalculateChecksum(b) {
		t.Fatalf("trailing whitespace should not change checksum")
	}
}

func TestCalculateChecksumChangesWithIndentation(t *testing.T) {
	a := []byte("spec:\n  field: 1\n")
	b := []byte("spec:\n    field: 1\n")

	if CalculateChecksum(a) == CalculateChecksum(b) {
		t.Fatalf("meaningful indentation changes should change checksum")
	}
}

func TestCalculateChecksumChangesWithContent(t *testing.T) {
	a := []byte("value a")
	b := []byte("value b")

	if CalculateChecksum(a) == CalculateChecksum(b) {
		t.Fatalf("different content should change checksum")
	}
}
