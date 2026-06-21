package controller

import "github.com/charmbracelet/lipgloss"

// truncateToWidth shortens text to fit width display cells, appending an
// ellipsis when it had to cut.
func truncateToWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}

	if lipgloss.Width(text) <= width {
		return text
	}

	const ellipsis = "…"

	if width <= 1 {
		return ellipsis
	}

	maxWidth := width - lipgloss.Width(ellipsis)
	if maxWidth <= 0 {
		return ellipsis
	}

	currentWidth := 0

	result := make([]rune, 0, len(text))
	for _, r := range text {
		rWidth := lipgloss.Width(string(r))
		if currentWidth+rWidth > maxWidth {
			break
		}

		result = append(result, r)
		currentWidth += rWidth
	}

	return string(result) + ellipsis
}

// animateScroll returns a width-sized window into text that advances with offset,
// producing a marquee effect for text wider than width. Text that fits is
// returned unchanged.
func animateScroll(text string, width int, offset int) string {
	if width <= 0 {
		return ""
	}

	if lipgloss.Width(text) <= width {
		return text
	}

	const (
		gap   = "   " // gap between repeats
		pause = 5     // initial ticks before scrolling starts
	)

	if offset < pause {
		return truncateToWidth(text, width)
	}

	runes := []rune(text + gap)

	n := len(runes)
	if n == 0 {
		return ""
	}

	start := (offset - pause) % n

	res := make([]rune, 0, width)
	for i := range width {
		res = append(res, runes[(start+i)%n])
	}

	return string(res)
}
