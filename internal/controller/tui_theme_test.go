package controller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestThemeTokens(t *testing.T) {
	tests := []struct {
		name string
		got  lipgloss.CompleteAdaptiveColor
		want lipgloss.CompleteAdaptiveColor
	}{
		{"toxic", themeToxic, adaptive(brand("#B6F23A", "154", "10"), brand("#0C100D", "233", "0"))},
		{"orange", themeOrange, adaptive(brand("#F59E2C", "214", "3"), brand("#0C100D", "233", "0"))},
		{"accent", themeAccent, adaptive(brand("#00ADD8", "38", "6"), brand("#007E9E", "31", "6"))},
		{"error", themeError, adaptive(brand("#FF6B6B", "203", "9"), brand("#D93636", "167", "1"))},
		{"text", themeText, adaptive(brand("#EEF1EA", "255", "15"), brand("#0C100D", "233", "0"))},
		{"muted", themeMuted, adaptive(brand("#9AA39B", "247", "8"), brand("#5B625C", "241", "8"))},
		{"toxic fill", themeToxicFill, adaptive(lipgloss.CompleteColor{}, brand("#B6F23A", "154", "10"))},
		{"orange fill", themeOrangeFill, adaptive(lipgloss.CompleteColor{}, brand("#F59E2C", "214", "3"))},
		{"highlight", themeHighlight, adaptive(brand("#B6F23A", "154", "10"), brand("#0C100D", "233", "0"))},
		{"highlight text", themeHighlightText, adaptive(brand("#0C100D", "233", "0"), brand("#B6F23A", "154", "10"))},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %+v, want %+v", tt.name, tt.got, tt.want)
		}
	}
}

func TestThemeStatusStyle(t *testing.T) {
	tests := []struct {
		status string
		fg, bg lipgloss.TerminalColor
	}{
		{"killed", themeToxic, themeToxicFill},
		{"survived", themeOrange, themeOrangeFill},
		{"error", themeError, lipgloss.NoColor{}},
		{"not_covered", themeMuted, lipgloss.NoColor{}},
		{"timeout", themeMuted, lipgloss.NoColor{}},
	}

	for _, tt := range tests {
		style := themeStatusStyle(tt.status)
		if got := style.GetForeground(); got != tt.fg {
			t.Errorf("themeStatusStyle(%q) foreground = %+v, want %+v", tt.status, got, tt.fg)
		}

		if got := style.GetBackground(); got != tt.bg {
			t.Errorf("themeStatusStyle(%q) background = %+v, want %+v", tt.status, got, tt.bg)
		}

		if !style.GetBold() {
			t.Errorf("themeStatusStyle(%q) is not bold", tt.status)
		}
	}
}

func TestThemeSelectedStyle(t *testing.T) {
	style := themeSelectedStyle()
	if style.GetForeground() != themeHighlightText || style.GetBackground() != themeHighlight || !style.GetBold() {
		t.Fatalf("themeSelectedStyle = fg %+v bg %+v bold %v, want highlight text on highlight, bold",
			style.GetForeground(), style.GetBackground(), style.GetBold())
	}
}

func TestThemeTitleStyle(t *testing.T) {
	style := themeTitleStyle()
	if style.GetForeground() != themeTitle || !style.GetBold() {
		t.Fatalf("themeTitleStyle = fg %+v bold %v, want title, bold", style.GetForeground(), style.GetBold())
	}

	if top, right, bottom, left := style.GetPadding(); top != 1 || right != 0 || bottom != 0 || left != 2 {
		t.Fatalf("themeTitleStyle padding = %d %d %d %d, want 1 0 0 2", top, right, bottom, left)
	}

	if themeTitle != adaptive(brand("#B6F23A", "154", "10"), brand("#0C100D", "233", "0")) {
		t.Fatalf("themeTitle = %+v, want toxic on dark, ink on light", themeTitle)
	}
}

func TestThemeHeaderStyle(t *testing.T) {
	style := themeHeaderStyle()
	if style.GetForeground() != themeMuted || !style.GetBold() {
		t.Fatalf("themeHeaderStyle = fg %+v bold %v, want muted, bold", style.GetForeground(), style.GetBold())
	}
}

// TestNoInlineTUIColors keeps every TUI color in the theme, so the brand
// palette is changed in one place.
func TestNoInlineTUIColors(t *testing.T) {
	files, err := filepath.Glob("tui*.go")
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		if file == "tui_theme.go" || strings.HasSuffix(file, "_test.go") {
			continue
		}

		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(string(src), "lipgloss.Color(") {
			t.Errorf("%s sets a color inline; use a token from tui_theme.go", file)
		}
	}
}
