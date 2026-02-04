package cli

import (
	"strings"
	"testing"
)

func TestUsageIncludesUsageHeader(t *testing.T) {
	text := Usage()
	if !strings.Contains(text, "Usage:") {
		t.Fatalf("Usage() missing Usage header")
	}
	if !strings.Contains(text, "ec") {
		t.Fatalf("Usage() missing ec command")
	}
}
