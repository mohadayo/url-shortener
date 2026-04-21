package main

import (
	"strings"
	"testing"
)

func TestValidateURLLengthLimit(t *testing.T) {
	// 2048文字ちょうどのURLは許可される
	longPath := strings.Repeat("a", 2048-len("https://example.com/"))
	validLongURL := "https://example.com/" + longPath
	if err := validateURL(validLongURL); err != nil {
		t.Errorf("URL at max length should be valid, got error: %v", err)
	}

	// 2049文字のURLは拒否される
	tooLongURL := validLongURL + "x"
	if err := validateURL(tooLongURL); err == nil {
		t.Error("URL exceeding max length should be rejected")
	}
}

func TestValidateURLVeryLongURL(t *testing.T) {
	// 極端に長いURL（10000文字）でDoS攻撃を模擬
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
