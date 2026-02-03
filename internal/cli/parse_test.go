package cli

import (
	"errors"
	"testing"
)

func TestParseBackupDefault(t *testing.T) {
	opts, err := Parse([]string{})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.Backup {
		t.Fatalf("Parse() Backup = true, want false")
	}
}

func TestParseBackupFlag(t *testing.T) {
	args := []string{"--backup", "--base", "b", "--local", "l", "--remote", "r", "--merged", "m"}
	opts, err := Parse(args)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !opts.Backup {
		t.Fatalf("Parse() Backup = false, want true")
	}
}

func TestParseVersionFlag(t *testing.T) {
	_, err := Parse([]string{"--version"})
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("Parse() error = %v, want ErrVersion", err)
	}
}
