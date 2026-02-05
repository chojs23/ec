package tui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLoadThemeFromConfigMissingFileUsesDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	theme, err := loadThemeFromConfig()
	if err != nil {
		t.Fatalf("loadThemeFromConfig() error = %v", err)
	}
	if theme.Name != "default" {
		t.Fatalf("theme name = %q, want default", theme.Name)
	}
	if theme.HeaderBg != "62" {
		t.Fatalf("header_bg = %q, want 62", theme.HeaderBg)
	}
}

func TestLoadThemeFromConfigMergesOverrides(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)

	configPath := filepath.Join(configDir, "ec", themeConfigFileName)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}

	config := `{
  "default": "warm",
  "themes": {
    "warm": {
      "header_bg": "94",
      "dim_foreground_muted": "123"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	theme, err := loadThemeFromConfig()
	if err != nil {
		t.Fatalf("loadThemeFromConfig() error = %v", err)
	}
	if theme.Name != "warm" {
		t.Fatalf("theme name = %q, want warm", theme.Name)
	}
	if theme.HeaderBg != "94" {
		t.Fatalf("header_bg = %q, want 94", theme.HeaderBg)
	}
	if theme.HeaderFg != "230" {
		t.Fatalf("header_fg = %q, want 230", theme.HeaderFg)
	}
	if theme.DimForegroundMuted != "123" {
		t.Fatalf("dim_foreground_muted = %q, want 123", theme.DimForegroundMuted)
	}
}

func TestLoadThemeFromConfigMissingThemeReturnsError(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)

	configPath := filepath.Join(configDir, "ec", themeConfigFileName)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}

	config := `{
  "default": "missing",
  "themes": {
    "warm": {
      "header_bg": "94"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadThemeFromConfig()
	if err == nil {
		t.Fatal("loadThemeFromConfig() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want missing theme error", err.Error())
	}
}

func TestLoadThemeFromConfigInvalidJSONReturnsError(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)

	configPath := filepath.Join(configDir, "ec", themeConfigFileName)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(configPath, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadThemeFromConfig()
	if err == nil {
		t.Fatal("loadThemeFromConfig() error = nil, want error")
	}
}

func TestApplyThemeUpdatesDimColors(t *testing.T) {
	resetThemeForTest()
	t.Cleanup(resetThemeForTest)

	theme := defaultTheme()
	theme.DimForegroundLight = "101"
	theme.DimForegroundDark = "102"
	theme.DimForegroundMuted = "103"
	theme.SelectedHunkBg = "104"

	applyTheme(theme)

	if dimForegroundLight != lipgloss.Color("101") {
		t.Fatalf("dimForegroundLight = %q, want 101", dimForegroundLight)
	}
	if dimForegroundDark != lipgloss.Color("102") {
		t.Fatalf("dimForegroundDark = %q, want 102", dimForegroundDark)
	}
	if dimForegroundMuted != lipgloss.Color("103") {
		t.Fatalf("dimForegroundMuted = %q, want 103", dimForegroundMuted)
	}
	if selectedHunkBackground != lipgloss.Color("104") {
		t.Fatalf("selectedHunkBackground = %q, want 104", selectedHunkBackground)
	}
}

func TestEnsureThemeLoadedAppliesConfigOnce(t *testing.T) {
	resetThemeForTest()
	t.Cleanup(resetThemeForTest)

	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)

	configPath := filepath.Join(configDir, "ec", themeConfigFileName)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}

	config := `{
  "default": "first",
  "themes": {
    "first": {
      "selected_hunk_bg": "111"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureThemeLoaded(); err != nil {
		t.Fatalf("ensureThemeLoaded() error = %v", err)
	}
	if selectedHunkBackground != lipgloss.Color("111") {
		t.Fatalf("selectedHunkBackground = %q, want 111", selectedHunkBackground)
	}

	config = `{
  "default": "second",
  "themes": {
    "second": {
      "selected_hunk_bg": "222"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureThemeLoaded(); err != nil {
		t.Fatalf("ensureThemeLoaded() error = %v", err)
	}
	if selectedHunkBackground != lipgloss.Color("111") {
		t.Fatalf("selectedHunkBackground = %q, want 111", selectedHunkBackground)
	}
}

func TestEnsureThemeLoadedReturnsError(t *testing.T) {
	resetThemeForTest()
	t.Cleanup(resetThemeForTest)

	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)

	configPath := filepath.Join(configDir, "ec", themeConfigFileName)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(configPath, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureThemeLoaded(); err == nil {
		t.Fatal("ensureThemeLoaded() error = nil, want error")
	}
}

func resetThemeForTest() {
	themeOnce = sync.Once{}
	themeErr = nil
	applyTheme(defaultTheme())
}
