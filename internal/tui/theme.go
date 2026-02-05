package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

const themeConfigFileName = "themes.json"

type ThemeConfig struct {
	Default string           `json:"default"`
	Themes  map[string]Theme `json:"themes"`
}

type Theme struct {
	Name string `json:"-"`

	TitleFg                string `json:"title_fg"`
	PaneBorder             string `json:"pane_border"`
	SelectedPaneBorder     string `json:"selected_pane_border"`
	SidePaneBorder         string `json:"side_pane_border"`
	SelectedSideBorder     string `json:"selected_side_border"`
	HeaderBg               string `json:"header_bg"`
	HeaderFg               string `json:"header_fg"`
	FooterBg               string `json:"footer_bg"`
	FooterFg               string `json:"footer_fg"`
	LineNumberFg           string `json:"line_number"`
	OursHighlightBg        string `json:"ours_highlight_bg"`
	OursHighlightFg        string `json:"ours_highlight_fg"`
	TheirsHighlightBg      string `json:"theirs_highlight_bg"`
	TheirsHighlightFg      string `json:"theirs_highlight_fg"`
	ResultFg               string `json:"result_fg"`
	ResultHighlightBg      string `json:"result_highlight_bg"`
	ResultHighlightFg      string `json:"result_highlight_fg"`
	ModifiedBg             string `json:"modified_bg"`
	ModifiedFg             string `json:"modified_fg"`
	AddedBg                string `json:"added_bg"`
	AddedFg                string `json:"added_fg"`
	RemovedBg              string `json:"removed_bg"`
	RemovedFg              string `json:"removed_fg"`
	ConflictedBg           string `json:"conflicted_bg"`
	ConflictedFg           string `json:"conflicted_fg"`
	InsertMarkerFg         string `json:"insert_marker_fg"`
	SelectedHunkMarkerFg   string `json:"selected_hunk_marker_fg"`
	SelectedHunkMarkerBg   string `json:"selected_hunk_marker_bg"`
	SelectedHunkBg         string `json:"selected_hunk_bg"`
	StatusResolvedFg       string `json:"status_resolved_fg"`
	StatusUnresolvedFg     string `json:"status_unresolved_fg"`
	ResultResolvedFg       string `json:"result_resolved_marker_fg"`
	ResultResolvedBorder   string `json:"result_resolved_border"`
	ResultUnresolvedBorder string `json:"result_unresolved_border"`
	ToastBg                string `json:"toast_bg"`
	ToastFg                string `json:"toast_fg"`
	SelectorResolvedFg     string `json:"selector_resolved_fg"`
	SelectorUnresolvedFg   string `json:"selector_unresolved_fg"`
	DimForegroundLight     string `json:"dim_foreground_light"`
	DimForegroundDark      string `json:"dim_foreground_dark"`
	DimForegroundMuted     string `json:"dim_foreground_muted"`
}

var (
	themeOnce sync.Once
	themeErr  error
)

func init() {
	applyTheme(defaultTheme())
}

func ensureThemeLoaded() error {
	themeOnce.Do(func() {
		theme, err := loadThemeFromConfig()
		if err != nil {
			themeErr = err
			return
		}
		applyTheme(theme)
	})
	return themeErr
}

func loadThemeFromConfig() (Theme, error) {
	fallback := defaultTheme()
	configPath, err := themeConfigPath()
	if err != nil {
		return fallback, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fallback, nil
		}
		return Theme{}, fmt.Errorf("read theme config: %w", err)
	}

	var cfg ThemeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Theme{}, fmt.Errorf("parse theme config: %w", err)
	}

	themeName := strings.TrimSpace(cfg.Default)
	if themeName == "" {
		themeName = "default"
	}

	theme, ok := cfg.Themes[themeName]
	if !ok {
		return Theme{}, fmt.Errorf("theme %q not found in %s", themeName, configPath)
	}
	theme.Name = themeName
	return mergeTheme(fallback, theme), nil
}

func themeConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "ec", themeConfigFileName), nil
}

func defaultTheme() Theme {
	return Theme{
		Name:                   "default",
		TitleFg:                "170",
		PaneBorder:             "63",
		SelectedPaneBorder:     "205",
		SidePaneBorder:         "255",
		SelectedSideBorder:     "33",
		HeaderBg:               "62",
		HeaderFg:               "230",
		FooterBg:               "236",
		FooterFg:               "243",
		LineNumberFg:           "241",
		OursHighlightBg:        "24",
		OursHighlightFg:        "230",
		TheirsHighlightBg:      "52",
		TheirsHighlightFg:      "230",
		ResultFg:               "231",
		ResultHighlightBg:      "60",
		ResultHighlightFg:      "230",
		ModifiedBg:             "24",
		ModifiedFg:             "231",
		AddedBg:                "28",
		AddedFg:                "231",
		RemovedBg:              "237",
		RemovedFg:              "250",
		ConflictedBg:           "131",
		ConflictedFg:           "231",
		InsertMarkerFg:         "196",
		SelectedHunkMarkerFg:   "226",
		SelectedHunkMarkerBg:   "88",
		SelectedHunkBg:         "236",
		StatusResolvedFg:       "42",
		StatusUnresolvedFg:     "196",
		ResultResolvedFg:       "42",
		ResultResolvedBorder:   "42",
		ResultUnresolvedBorder: "196",
		ToastBg:                "22",
		ToastFg:                "230",
		SelectorResolvedFg:     "42",
		SelectorUnresolvedFg:   "196",
		DimForegroundLight:     "231",
		DimForegroundDark:      "16",
		DimForegroundMuted:     "244",
	}
}

func mergeTheme(base Theme, override Theme) Theme {
	return Theme{
		Name:                   override.Name,
		TitleFg:                pickColor(base.TitleFg, override.TitleFg),
		PaneBorder:             pickColor(base.PaneBorder, override.PaneBorder),
		SelectedPaneBorder:     pickColor(base.SelectedPaneBorder, override.SelectedPaneBorder),
		SidePaneBorder:         pickColor(base.SidePaneBorder, override.SidePaneBorder),
		SelectedSideBorder:     pickColor(base.SelectedSideBorder, override.SelectedSideBorder),
		HeaderBg:               pickColor(base.HeaderBg, override.HeaderBg),
		HeaderFg:               pickColor(base.HeaderFg, override.HeaderFg),
		FooterBg:               pickColor(base.FooterBg, override.FooterBg),
		FooterFg:               pickColor(base.FooterFg, override.FooterFg),
		LineNumberFg:           pickColor(base.LineNumberFg, override.LineNumberFg),
		OursHighlightBg:        pickColor(base.OursHighlightBg, override.OursHighlightBg),
		OursHighlightFg:        pickColor(base.OursHighlightFg, override.OursHighlightFg),
		TheirsHighlightBg:      pickColor(base.TheirsHighlightBg, override.TheirsHighlightBg),
		TheirsHighlightFg:      pickColor(base.TheirsHighlightFg, override.TheirsHighlightFg),
		ResultFg:               pickColor(base.ResultFg, override.ResultFg),
		ResultHighlightBg:      pickColor(base.ResultHighlightBg, override.ResultHighlightBg),
		ResultHighlightFg:      pickColor(base.ResultHighlightFg, override.ResultHighlightFg),
		ModifiedBg:             pickColor(base.ModifiedBg, override.ModifiedBg),
		ModifiedFg:             pickColor(base.ModifiedFg, override.ModifiedFg),
		AddedBg:                pickColor(base.AddedBg, override.AddedBg),
		AddedFg:                pickColor(base.AddedFg, override.AddedFg),
		RemovedBg:              pickColor(base.RemovedBg, override.RemovedBg),
		RemovedFg:              pickColor(base.RemovedFg, override.RemovedFg),
		ConflictedBg:           pickColor(base.ConflictedBg, override.ConflictedBg),
		ConflictedFg:           pickColor(base.ConflictedFg, override.ConflictedFg),
		InsertMarkerFg:         pickColor(base.InsertMarkerFg, override.InsertMarkerFg),
		SelectedHunkMarkerFg:   pickColor(base.SelectedHunkMarkerFg, override.SelectedHunkMarkerFg),
		SelectedHunkMarkerBg:   pickColor(base.SelectedHunkMarkerBg, override.SelectedHunkMarkerBg),
		SelectedHunkBg:         pickColor(base.SelectedHunkBg, override.SelectedHunkBg),
		StatusResolvedFg:       pickColor(base.StatusResolvedFg, override.StatusResolvedFg),
		StatusUnresolvedFg:     pickColor(base.StatusUnresolvedFg, override.StatusUnresolvedFg),
		ResultResolvedFg:       pickColor(base.ResultResolvedFg, override.ResultResolvedFg),
		ResultResolvedBorder:   pickColor(base.ResultResolvedBorder, override.ResultResolvedBorder),
		ResultUnresolvedBorder: pickColor(base.ResultUnresolvedBorder, override.ResultUnresolvedBorder),
		ToastBg:                pickColor(base.ToastBg, override.ToastBg),
		ToastFg:                pickColor(base.ToastFg, override.ToastFg),
		SelectorResolvedFg:     pickColor(base.SelectorResolvedFg, override.SelectorResolvedFg),
		SelectorUnresolvedFg:   pickColor(base.SelectorUnresolvedFg, override.SelectorUnresolvedFg),
		DimForegroundLight:     pickColor(base.DimForegroundLight, override.DimForegroundLight),
		DimForegroundDark:      pickColor(base.DimForegroundDark, override.DimForegroundDark),
		DimForegroundMuted:     pickColor(base.DimForegroundMuted, override.DimForegroundMuted),
	}
}

func pickColor(base string, override string) string {
	if override != "" {
		return override
	}
	return base
}

func applyTheme(theme Theme) {
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.TitleFg)).
		Padding(0, 1)

	paneStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.PaneBorder)).
		Padding(0, 1)

	selectedPaneStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.SelectedPaneBorder)).
		Padding(0, 1)

	oursPaneStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.SidePaneBorder)).
		Padding(0, 1)

	theirsPaneStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.SidePaneBorder)).
		Padding(0, 1)

	selectedSidePaneStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.SelectedSideBorder)).
		Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
		Bold(true).
		Background(lipgloss.Color(theme.HeaderBg)).
		Foreground(lipgloss.Color(theme.HeaderFg)).
		Padding(0, 2)

	footerStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(theme.FooterBg)).
		Foreground(lipgloss.Color(theme.FooterFg)).
		Padding(0, 2)

	lineNumberStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.LineNumberFg))

	oursHighlightStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(theme.OursHighlightBg)).
		Foreground(lipgloss.Color(theme.OursHighlightFg))

	theirsHighlightStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(theme.TheirsHighlightBg)).
		Foreground(lipgloss.Color(theme.TheirsHighlightFg))

	resultLineStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ResultFg))

	resultHighlightStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(theme.ResultHighlightBg)).
		Foreground(lipgloss.Color(theme.ResultHighlightFg))

	modifiedLineStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(theme.ModifiedBg)).
		Foreground(lipgloss.Color(theme.ModifiedFg))

	addedLineStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(theme.AddedBg)).
		Foreground(lipgloss.Color(theme.AddedFg))

	removedLineStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(theme.RemovedBg)).
		Foreground(lipgloss.Color(theme.RemovedFg))

	conflictedLineStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(theme.ConflictedBg)).
		Foreground(lipgloss.Color(theme.ConflictedFg))

	insertMarkerStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.InsertMarkerFg)).
		Bold(true)

	selectedHunkMarkerStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.SelectedHunkMarkerFg)).
		Background(lipgloss.Color(theme.SelectedHunkMarkerBg)).
		Bold(true)

	selectedHunkBackground = lipgloss.Color(theme.SelectedHunkBg)

	statusResolvedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.StatusResolvedFg)).
		Bold(true)

	statusUnresolvedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.StatusUnresolvedFg)).
		Bold(true)

	resultResolvedMarkerStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ResultResolvedFg)).
		Bold(true)

	resultResolvedPaneStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.ResultResolvedBorder)).
		Padding(0, 1)

	resultUnresolvedPaneStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.ResultUnresolvedBorder)).
		Padding(0, 1)

	toastStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(theme.ToastBg)).
		Foreground(lipgloss.Color(theme.ToastFg)).
		Padding(0, 1)

	toastLineStyle = lipgloss.NewStyle().
		Align(lipgloss.Right).
		Padding(0, 2)

	resultTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Background(lipgloss.Color(theme.HeaderBg)).
		Foreground(lipgloss.Color(theme.HeaderFg)).
		Padding(0, 2)

	resolvedLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.SelectorResolvedFg))
	unresolvedLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.SelectorUnresolvedFg))

	dimForegroundLight = lipgloss.Color(theme.DimForegroundLight)
	dimForegroundDark = lipgloss.Color(theme.DimForegroundDark)
	dimForegroundMuted = lipgloss.Color(theme.DimForegroundMuted)
}
