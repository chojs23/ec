package cli

import "testing"

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
