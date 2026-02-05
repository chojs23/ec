package main

import "testing"

func TestVersionStringOverride(t *testing.T) {
	old := version
	version = "v1.2.3"
	t.Cleanup(func() {
		version = old
	})

	if got := versionString(); got != "v1.2.3" {
		t.Fatalf("versionString() = %q, want %q", got, "v1.2.3")
	}
}
