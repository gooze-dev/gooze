package controller

import "github.com/charmbracelet/lipgloss"

// The Gooze brand theme for the TUI (design system: TUITheme). Every color
// has a true-color value, a 256-color and a 16-color fallback, and a dark
// and a light variant picked from the terminal's background. Toxic and Gopher
// Blue never sit as text on a light background: there toxic becomes a fill
// behind ink text and Gopher Blue its deep shade. The brand has no purple.
var (
	// themeToxic is killed mutants and mutated code.
	themeToxic = adaptive(brand("#B6F23A", "154", "10"), brand("#0C100D", "233", "0"))
	// themeToxicFill sits behind themeToxic text on a light background.
	themeToxicFill = adaptive(lipgloss.CompleteColor{}, brand("#B6F23A", "154", "10"))
	// themeOrange is survived mutants.
	themeOrange = adaptive(brand("#F59E2C", "214", "3"), brand("#0C100D", "233", "0"))
	// themeOrangeFill sits behind themeOrange text on a light background.
	themeOrangeFill = adaptive(lipgloss.CompleteColor{}, brand("#F59E2C", "214", "3"))
	// themeAccent is Gopher Blue: original code, paths, numbers, borders.
	themeAccent = adaptive(brand("#00ADD8", "38", "6"), brand("#007E9E", "31", "6"))
	// themeError is mutants that errored.
	themeError = adaptive(brand("#FF6B6B", "203", "9"), brand("#D93636", "167", "1"))
	// themeText is body text.
	themeText = adaptive(brand("#EEF1EA", "255", "15"), brand("#0C100D", "233", "0"))
	// themeMuted is captions, idle borders and statuses without a verdict.
	themeMuted = adaptive(brand("#9AA39B", "247", "8"), brand("#5B625C", "241", "8"))
	// themeTitle is view titles: toxic on dark, ink on light.
	themeTitle = themeToxic
	// themeHighlight fills the selected row: toxic on dark, ink on light.
	themeHighlight = adaptive(brand("#B6F23A", "154", "10"), brand("#0C100D", "233", "0"))
	// themeHighlightText is text on themeHighlight.
	themeHighlightText = adaptive(brand("#0C100D", "233", "0"), brand("#B6F23A", "154", "10"))
)

func brand(trueColor, ansi256, ansi string) lipgloss.CompleteColor {
	return lipgloss.CompleteColor{TrueColor: trueColor, ANSI256: ansi256, ANSI: ansi}
}

func adaptive(dark, light lipgloss.CompleteColor) lipgloss.CompleteAdaptiveColor {
	return lipgloss.CompleteAdaptiveColor{Dark: dark, Light: light}
}

// themeTitleStyle is the style of a view's title line.
func themeTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(themeTitle).Bold(true).Padding(1, 0, 0, 2)
}

// themeHeaderStyle is the style of column headers and box headers.
func themeHeaderStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(themeMuted).Bold(true)
}

// themeSelectedStyle is the style of the selected list row.
func themeSelectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(themeHighlightText).Background(themeHighlight).Bold(true)
}

// themeStatusStyle is the style of a mutation status label.
func themeStatusStyle(status string) lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true)

	switch status {
	case "killed":
		return style.Foreground(themeToxic).Background(themeToxicFill)
	case "survived":
		return style.Foreground(themeOrange).Background(themeOrangeFill)
	case "error":
		return style.Foreground(themeError)
	default:
		return style.Foreground(themeMuted)
	}
}
