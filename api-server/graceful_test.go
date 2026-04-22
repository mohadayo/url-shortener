package main

import (
	"strings"
	"testing"
)

func TestValidateURLLengthLimit(t *testing.T) {
	longPath := strings.Repeat("a", 2048-len("https://example.com/"))
	validLongURL := "https://example.com/" + longPath
	if err := validateURL(validLongURL); err != nil {
		t.Errorf("URL at max length should be valid, got error: %v", err)
	}

	tooLongURL := validLongURL + "x"
	if err := validateURL(tooLongURL); err == nil {
		t.Error("URL exceeding max length should be rejected")
	}
}

func TestValidateURLVeryLongURL(t *testing.T) {
	hugeURL := "https://example.com/" + strings.Repeat("a", 10000)
	err := validateURL(hugeURL)
	if err == nil {
		t.Error("extremely long URL should be rejected")
	}
}

func TestMaxURLLengthConstant(t *testing.T) {
	if maxURLLength != 2048 {
		t.Errorf("expected maxURLLength=2048, got %d", maxURLLength)
	}
}

func TestValidShortCode(t *testing.T) {
	tests := []struct {
		code  string
		valid bool
	}{
		{"abcdef12", true},
		{"00112233", true},
		{"aabbccdd", true},
		{"", false},
		{"short", false},
		{"toolongcode123", false},
		{"ABCDEF12", false},
		{"abcdefg!", false},
		{"../../../", false},
		{"abcdefgh", false},
	}
	for _, tc := range tests {
		got := validShortCode.MatchString(tc.code)
		if got != tc.valid {
			t.Errorf("validShortCode(%q) = %v, want %v", tc.code, got, tc.valid)
		}
	}
}
