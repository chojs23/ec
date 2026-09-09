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
	if theme.HeaderBg != "#161b22" {
		t.Fatalf("header_bg = %q, want #161b22", theme.HeaderBg)
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
	  "file_status_modified_fg": "202",
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
	if theme.HeaderBg != "94" {
		t.Fatalf("header_bg = %q, want 94", theme.HeaderBg)
	}
	if theme.HeaderFg != "#f0f6fc" {
		t.Fatalf("header_fg = %q, want #f0f6fc", theme.HeaderFg)
	}
	if theme.DimForegroundMuted != "123" {
		t.Fatalf("dim_foreground_muted = %q, want 123", theme.DimForegroundMuted)
	}
	if theme.FileStatusModifiedFg != "202" || theme.FileStatusDeletedFg != "#f85149" {
		t.Fatalf("file status colors = %q and %q, want override and fallback", theme.FileStatusModifiedFg, theme.FileStatusDeletedFg)
	}
}

func TestDefaultThemeUsesDistinctPaneAndDiffColors(t *testing.T) {
	theme := defaultTheme()
	if theme.TitleFg != "#c9d1d9" || theme.HeaderBg != "#161b22" || theme.FooterBg != "#161b22" {
		t.Fatalf("default chrome colors = %q, %q, and %q, want neutral GitHub-style chrome", theme.TitleFg, theme.HeaderBg, theme.FooterBg)
	}
	if theme.HeaderFg != "#f0f6fc" || theme.FooterFg != "#8b949e" || theme.LineNumberFg != "#6e7681" {
		t.Fatalf("default text colors = %q, %q, and %q, want clear primary and muted text", theme.HeaderFg, theme.FooterFg, theme.LineNumberFg)
	}
	if theme.PaneBorder != "245" || theme.SidePaneBorder != "245" {
		t.Fatalf("default borders = %q and %q, want gray", theme.PaneBorder, theme.SidePaneBorder)
	}
	if theme.SelectedPaneBorder != "117" || theme.SelectedSideBorder != "117" {
		t.Fatalf("selected borders = %q and %q, want light blue", theme.SelectedPaneBorder, theme.SelectedSideBorder)
	}
	if theme.ResultResolvedBorder != theme.PaneBorder || theme.ResultUnresolvedBorder != theme.PaneBorder {
		t.Fatalf("result borders = %q and %q, want neutral pane border %q", theme.ResultResolvedBorder, theme.ResultUnresolvedBorder, theme.PaneBorder)
	}
	if theme.SelectedHunkMarkerFg != "117" || theme.SelectedHunkMarkerBg != theme.HeaderBg {
		t.Fatalf("active block marker = %q on %q, want light blue on neutral chrome", theme.SelectedHunkMarkerFg, theme.SelectedHunkMarkerBg)
	}
	if theme.AddedFg != "#7ee787" || theme.AddedBg != "#0d4429" {
		t.Fatalf("added colors = %q on %q, want GitHub-style green", theme.AddedFg, theme.AddedBg)
	}
	if theme.RemovedFg != "#ff7b72" || theme.RemovedBg != "#4c1c1c" {
		t.Fatalf("removed colors = %q on %q, want GitHub-style red", theme.RemovedFg, theme.RemovedBg)
	}
	if theme.DiffHunkFg != "#79c0ff" || theme.DiffHunkBg != "#162a46" {
		t.Fatalf("hunk colors = %q on %q, want muted GitHub-style blue", theme.DiffHunkFg, theme.DiffHunkBg)
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
	theme.FileStatusModifiedFg = "105"
	theme.DiffHunkBg = "106"
	theme.DiffHunkFg = "107"

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
	if fileStatusModifiedStyle.GetForeground() != lipgloss.Color("105") {
		t.Fatalf("fileStatusModifiedStyle foreground = %q, want 105", fileStatusModifiedStyle.GetForeground())
	}
	if diffHunkStyle.GetBackground() != lipgloss.Color("106") || diffHunkStyle.GetForeground() != lipgloss.Color("107") {
		t.Fatalf("diffHunkStyle colors = %q on %q, want 107 on 106", diffHunkStyle.GetForeground(), diffHunkStyle.GetBackground())
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
