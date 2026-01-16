package controller

import (
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	m "github.com/mouse-blink/gooze/internal/model"
	"golang.org/x/term"
)

const (
	// ANSI color codes for zero values (dark gray, faint).
	grayColor  = "\033[2;90m" // Faint + dark gray
	resetColor = "\033[0m"    // Reset
)

// TUI implements UI using Bubble Tea for interactive display.
type TUI struct {
	output io.Writer
}

// NewTUI creates a new TUI.
func NewTUI(output io.Writer) *TUI {
	return &TUI{output: output}
}

// ShowNotImplemented displays a "not implemented" message using TUI.
func (p *TUI) ShowNotImplemented(count int) error {
	model := newNotImplementedModel(count)

	// Not implemented message is always short, just print and exit
	_, err := fmt.Fprint(p.output, model.View())

	return err
}

// DisplayMutationEstimations shows pre-calculated mutation estimations using TUI.
func (p *TUI) DisplayMutationEstimations(estimations map[m.Path]MutationEstimation) error {
	// Convert map to sorted slice for display
	counts := make([]mutationCount, 0, len(estimations))
	totalArithmetic := 0
	totalBoolean := 0

	// Sort paths for consistent output
	paths := make([]m.Path, 0, len(estimations))
	for path := range estimations {
		paths = append(paths, path)
	}

	// Simple sort by string comparison
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			if string(paths[i]) > string(paths[j]) {
				paths[i], paths[j] = paths[j], paths[i]
			}
		}
	}

	for _, path := range paths {
		est := estimations[path]
		counts = append(counts, mutationCount{
			file:       string(path),
			arithmetic: est.Arithmetic,
			boolean:    est.Boolean,
		})
		totalArithmetic += est.Arithmetic
		totalBoolean += est.Boolean
	}

	model := newMutationCountModel(counts, totalArithmetic, totalBoolean)

	// Get initial terminal size
	if f, ok := p.output.(*os.File); ok {
		width, height, err := term.GetSize(int(f.Fd()))
		if err == nil {
			model.height = height
			model.width = width
		}
	}

	// If list is small, just print and exit
	if !model.needsPagination() {
		_, err := fmt.Fprint(p.output, model.View())
		return err
	}

	program := tea.NewProgram(model, tea.WithOutput(p.output), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return err
	}

	return nil
}

// mutationCount holds the mutation count for a single file.
type mutationCount struct {
	file       string
	arithmetic int
	boolean    int
}

// notImplementedModel represents a simple model for showing "not implemented" message.
type notImplementedModel struct {
	count int
}

func newNotImplementedModel(count int) notImplementedModel {
	return notImplementedModel{
		count: count,
	}
}

func (nm notImplementedModel) View() string {
	var b strings.Builder

	b.WriteString("╔════════════════════════════════════════════════════════════════╗\n")
	b.WriteString("║                    Gooze - Mutation Testing                    ║\n")
	b.WriteString("╚════════════════════════════════════════════════════════════════╝\n\n")

	fmt.Fprintf(&b, "  📊 Found %d source file(s)\n\n", nm.count)
	b.WriteString("  ⚠️  Mutation testing not yet implemented.\n")
	b.WriteString("  💡 Use --list or -l flag to see mutation counts.\n\n")

	return b.String()
}

// mutationCountModel represents the Bubble Tea model for displaying mutation counts.
type mutationCountModel struct {
	counts          []mutationCount
	totalArithmetic int
	totalBoolean    int
	height          int
	width           int
	offset          int // Current scroll offset
	quitting        bool
}

func newMutationCountModel(counts []mutationCount, totalArithmetic int, totalBoolean int) mutationCountModel {
	return mutationCountModel{
		counts:          counts,
		totalArithmetic: totalArithmetic,
		totalBoolean:    totalBoolean,
		height:          0, // Will be set on first WindowSizeMsg
		width:           0,
		offset:          0,
		quitting:        false,
	}
}

func (mcm mutationCountModel) Init() tea.Cmd {
	return nil
}

func (mcm mutationCountModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		mcm.height = msg.Height
		mcm.width = msg.Width

		return mcm, nil

	case tea.KeyMsg:
		return mcm.handleKeyPress(msg)
	}

	return mcm, nil
}

//nolint:cyclop,exhaustive // Key handling requires multiple cases for UI navigation
func (mcm mutationCountModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		mcm.quitting = true
		return mcm, tea.Quit
	default:
		// Handle other key types in the string switch below
	}

	switch msg.String() {
	case "q":
		mcm.quitting = true
		return mcm, tea.Quit

	case "down", "j":
		mcm.offset++

		maxOffset := mcm.maxOffset()
		if mcm.offset > maxOffset {
			mcm.offset = maxOffset
		}

		return mcm, nil

	case "up", "k":
		mcm.offset--
		if mcm.offset < 0 {
			mcm.offset = 0
		}

		return mcm, nil

	case "g", "home":
		mcm.offset = 0

		return mcm, nil

	case "G", "end":
		mcm.offset = mcm.maxOffset()

		return mcm, nil

	case "d", "pgdown":
		mcm.offset += mcm.itemsPerPage()

		maxOffset := mcm.maxOffset()
		if mcm.offset > maxOffset {
			mcm.offset = maxOffset
		}

		return mcm, nil

	case "u", "pgup":
		mcm.offset -= mcm.itemsPerPage()
		if mcm.offset < 0 {
			mcm.offset = 0
		}

		return mcm, nil
	}

	return mcm, nil
}

// itemsPerPage calculates how many items can fit on screen.
func (mcm mutationCountModel) itemsPerPage() int {
	if mcm.height == 0 {
		return 10 // Default
	}
	// Reserve space for:
	// - Header: 4 lines (box + empty)
	// - Title: 2 lines (summary + empty)
	// - Total: 2 lines (empty + total)
	// - Footer: 3 lines (empty + page + help)
	// - Top margin: 1 line
	// Total: 12 lines
	reserved := 12

	available := mcm.height - reserved
	if available < 1 {
		return 1
	}

	return available
}

// maxOffset returns the maximum scroll offset.
func (mcm mutationCountModel) maxOffset() int {
	itemCount := len(mcm.counts)

	perPage := mcm.itemsPerPage()
	if perPage <= 0 {
		return 0
	}

	maxOff := itemCount - perPage
	if maxOff < 0 {
		return 0
	}

	return maxOff
}

// needsPagination returns true if the list is too large to fit on screen.
func (mcm mutationCountModel) needsPagination() bool {
	totalFiles := len(mcm.counts)
	if totalFiles == 0 {
		return false
	}

	itemsPerPage := mcm.itemsPerPage()

	return totalFiles > itemsPerPage && mcm.height > 0
}

func (mcm mutationCountModel) View() string {
	var b strings.Builder

	mcm.renderHeader(&b)

	if len(mcm.counts) == 0 {
		b.WriteString("  📭 No source files found\n")
		return b.String()
	}

	mcm.renderMutationCountList(&b)

	return b.String()
}

func (mcm mutationCountModel) renderHeader(b *strings.Builder) {
	b.WriteString("╔════════════════════════════════════════════════════════════════╗\n")
	b.WriteString("║                    Gooze - Mutation Testing                    ║\n")
	b.WriteString("╚════════════════════════════════════════════════════════════════╝\n\n")
}

func (mcm mutationCountModel) renderMutationCountList(b *strings.Builder) {
	totalFiles := len(mcm.counts)

	b.WriteString("  🔢 mutations summary:\n\n")

	// Calculate pagination
	itemsPerPage := mcm.itemsPerPage()
	needsPagination := totalFiles > itemsPerPage && mcm.height > 0

	start := mcm.offset

	end := start + itemsPerPage
	if end > totalFiles {
		end = totalFiles
	}

	if start >= totalFiles {
		start = totalFiles - 1
		if start < 0 {
			start = 0
		}
	}

	// Show items for current page
	displayCounts := mcm.counts

	if needsPagination {
		displayCounts = mcm.counts[start:end]
	}

	for _, mc := range displayCounts {
		arithmeticColor := ""
		booleanColor := ""

		if mc.arithmetic == 0 {
			arithmeticColor = grayColor
		}

		if mc.boolean == 0 {
			booleanColor = grayColor
		}

		fmt.Fprintf(b, "  %s: %s%d%s arithmetic, %s%d%s boolean\n",
			mc.file,
			arithmeticColor, mc.arithmetic, resetColor,
			booleanColor, mc.boolean, resetColor)
	}

	// Total count
	b.WriteString("\n")
	fmt.Fprintf(b, "  📊 Total: %d arithmetic mutations across %d file(s)\n", mcm.totalArithmetic, totalFiles)
	fmt.Fprintf(b, "  📊 Total: %d boolean mutations across %d file(s)\n", mcm.totalBoolean, totalFiles)

	// Footer with navigation help
	if needsPagination {
		b.WriteString("\n")

		currentPage := (mcm.offset / itemsPerPage) + 1
		totalPages := (totalFiles + itemsPerPage - 1) / itemsPerPage
		fmt.Fprintf(b, "  Page %d/%d | Showing %d-%d of %d\n",
			currentPage, totalPages, start+1, end, totalFiles)
		b.WriteString("  ↑/k: up | ↓/j: down | g: top | G: bottom | q: quit\n")
	}
}

// DisplayMutationResults displays mutation testing results using TUI.
func (p *TUI) DisplayMutationResults(sources []m.Source, fileResults map[m.Path]interface{}) error {
	// Sort sources for consistent output
	sortedSources := sortSources(sources)

	results, totalMutations, totalKilled, totalSurvived := processResults(sortedSources, fileResults)

	model := newMutationResultsModel(results, totalMutations, totalKilled, totalSurvived)

	// Get initial terminal size
	if f, ok := p.output.(*os.File); ok {
		width, height, err := term.GetSize(int(f.Fd()))
		if err == nil {
			model.height = height
			model.width = width
		}
	}

	// If list is small, just print and exit
	if !model.needsPagination() {
		_, err := fmt.Fprint(p.output, model.View())
		return err
	}

	program := tea.NewProgram(model, tea.WithOutput(p.output), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return err
	}

	return nil
}

func processResults(sortedSources []m.Source, fileResults map[m.Path]interface{}) ([]fileResult, int, int, int) {
	results := make([]fileResult, 0, len(sortedSources))
	totalKilled := 0
	totalSurvived := 0
	totalMutations := 0

	// Extract results for each file
	for _, source := range sortedSources {
		result := fileResults[source.Origin]
		if result == nil {
			continue
		}

		reports := extractReportsFromResult(result)

		fileKilled := 0
		fileSurvived := 0

		for _, report := range reports {
			if report.Killed {
				fileKilled++
			} else {
				fileSurvived++
			}
		}

		results = append(results, fileResult{
			file:     string(source.Origin),
			reports:  reports,
			killed:   fileKilled,
			survived: fileSurvived,
		})

		totalMutations += len(reports)
		totalKilled += fileKilled
		totalSurvived += fileSurvived
	}

	return results, totalMutations, totalKilled, totalSurvived
}

// fileResult holds mutation results for a single file.
type fileResult struct {
	file     string
	reports  []m.Report
	killed   int
	survived int
}

// mutationResultsModel represents the Bubble Tea model for displaying mutation results.
type mutationResultsModel struct {
	results        []fileResult
	totalMutations int
	totalKilled    int
	totalSurvived  int
	height         int
	width          int
	offset         int
	quitting       bool
}

func newMutationResultsModel(results []fileResult, total, killed, survived int) mutationResultsModel {
	return mutationResultsModel{
		results:        results,
		totalMutations: total,
		totalKilled:    killed,
		totalSurvived:  survived,
		height:         0,
		width:          0,
		offset:         0,
		quitting:       false,
	}
}

func (mrm mutationResultsModel) Init() tea.Cmd {
	return nil
}

func (mrm mutationResultsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		mrm.height = msg.Height
		mrm.width = msg.Width

		return mrm, nil

	case tea.KeyMsg:
		return mrm.handleKeyPress(msg)
	}

	return mrm, nil
}

func (mrm mutationResultsModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	//nolint:exhaustive // We only handle specific navigation keys
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		mrm.quitting = true
		return mrm, tea.Quit
	default:
		// Handle other key types in the string switch below
	}

	switch msg.String() {
	case "q":
		mrm.quitting = true
		return mrm, tea.Quit

	case "down", "j":
		return mrm.scrollDown(), nil

	case "up", "k":
		return mrm.scrollUp(), nil

	case "g", "home":
		mrm.offset = 0
		return mrm, nil

	case "G", "end":
		mrm.offset = mrm.maxOffset()
		return mrm, nil

	case "d", "pgdown":
		return mrm.scrollPageDown(), nil

	case "u", "pgup":
		return mrm.scrollPageUp(), nil
	}

	return mrm, nil
}

func (mrm mutationResultsModel) scrollDown() mutationResultsModel {
	mrm.offset++

	maxOffset := mrm.maxOffset()
	if mrm.offset > maxOffset {
		mrm.offset = maxOffset
	}

	return mrm
}

func (mrm mutationResultsModel) scrollUp() mutationResultsModel {
	mrm.offset--
	if mrm.offset < 0 {
		mrm.offset = 0
	}

	return mrm
}

func (mrm mutationResultsModel) scrollPageDown() mutationResultsModel {
	linesPerPage := mrm.itemsPerPage()
	targetLine := mrm.offset + linesPerPage
	maxOffset := mrm.maxOffset()

	if targetLine > maxOffset {
		targetLine = maxOffset
	}

	mrm.offset = targetLine

	return mrm
}

func (mrm mutationResultsModel) scrollPageUp() mutationResultsModel {
	linesPerPage := mrm.itemsPerPage()
	targetLine := mrm.offset - linesPerPage

	if targetLine < 0 {
		targetLine = 0
	}

	mrm.offset = targetLine

	return mrm
}

func (mrm mutationResultsModel) itemsPerPage() int {
	if mrm.height == 0 {
		return 10
	}
	// Reserved lines:
	// - Header box: 4 lines
	// - "Mutation Testing Results" + blank: 2 lines
	// - Summary section: 3 lines
	// - Footer (pagination): 3 lines
	// Total: 12 lines
	reserved := 12

	available := mrm.height - reserved
	if available < 1 {
		return 1
	}

	return available
}

// totalLines calculates the total number of display lines needed for all results.
func (mrm mutationResultsModel) totalLines() int {
	total := 0
	for _, fr := range mrm.results {
		total++                  // File header line
		total += len(fr.reports) // Mutation detail lines
	}

	return total
}

func (mrm mutationResultsModel) maxOffset() int {
	totalLines := mrm.totalLines()
	available := mrm.itemsPerPage()

	if totalLines <= available {
		return 0
	}

	return totalLines - available
}

func (mrm mutationResultsModel) needsPagination() bool {
	if len(mrm.results) == 0 || mrm.height == 0 {
		return false
	}

	// Calculate total lines needed for all content
	totalLines := 0
	for _, fr := range mrm.results {
		totalLines++                  // File header line
		totalLines += len(fr.reports) // Mutation detail lines
	}

	reserved := 12
	available := mrm.height - reserved

	return totalLines > available
}

func (mrm mutationResultsModel) View() string {
	var b strings.Builder

	mrm.renderHeader(&b)

	if len(mrm.results) == 0 {
		b.WriteString("  📭 No mutation results found\n")
		return b.String()
	}

	mrm.renderResultsList(&b)

	return b.String()
}

func (mrm mutationResultsModel) renderHeader(b *strings.Builder) {
	b.WriteString("╔════════════════════════════════════════════════════════════════╗\n")
	b.WriteString("║                    Gooze - Mutation Testing                    ║\n")
	b.WriteString("╚════════════════════════════════════════════════════════════════╝\n")
	b.WriteString("  🧬 Mutation Testing Results:\n\n")
}

func (mrm mutationResultsModel) renderResultsList(b *strings.Builder) {
	score := mrm.calculateScore()
	needsPagination := mrm.needsPagination()

	allLines := mrm.buildContentLines()
	visibleLines := mrm.applyPagination(allLines, needsPagination)

	mrm.writeLines(b, visibleLines)
	mrm.writeSummary(b, score)
	mrm.writeFooter(b, needsPagination, len(allLines), len(mrm.results))
}

func (mrm mutationResultsModel) calculateScore() float64 {
	if mrm.totalMutations > 0 {
		return float64(mrm.totalKilled) / float64(mrm.totalMutations) * 100
	}

	return 0.0
}

func (mrm mutationResultsModel) buildContentLines() []string {
	allLines := []string{}

	for _, fr := range mrm.results {
		statusIcon := "✓"
		if fr.survived > 0 {
			statusIcon = "✗"
		}

		allLines = append(allLines, fmt.Sprintf("  %s %s: %d mutations (killed: %d, survived: %d)",
			statusIcon, fr.file, len(fr.reports), fr.killed, fr.survived))

		for _, report := range fr.reports {
			if report.Killed {
				allLines = append(allLines, fmt.Sprintf("    ✓ %s - killed", report.MutationID))
			} else {
				allLines = append(allLines, fmt.Sprintf("    ✗ %s - survived", report.MutationID))
			}
		}
	}

	return allLines
}

func (mrm mutationResultsModel) applyPagination(allLines []string, needsPagination bool) []string {
	if !needsPagination {
		return allLines
	}

	available := mrm.itemsPerPage()
	start := mrm.offset
	end := start + available

	if start >= len(allLines) {
		start = len(allLines) - 1
		if start < 0 {
			start = 0
		}
	}

	if end > len(allLines) {
		end = len(allLines)
	}

	return allLines[start:end]
}

func (mrm mutationResultsModel) writeLines(b *strings.Builder, lines []string) {
	for _, line := range lines {
		fmt.Fprintf(b, "%s\n", line)
	}
}

func (mrm mutationResultsModel) writeSummary(b *strings.Builder, score float64) {
	b.WriteString("\n")
	fmt.Fprintf(b, "  📊 Summary:\n")
	fmt.Fprintf(b, "  Total: %d | Killed: %d | Survived: %d | Score: %.1f%%\n",
		mrm.totalMutations, mrm.totalKilled, mrm.totalSurvived, score)
}

func (mrm mutationResultsModel) writeFooter(b *strings.Builder, needsPagination bool, totalLines, totalFiles int) {
	if !needsPagination {
		return
	}

	b.WriteString("\n")

	available := mrm.itemsPerPage()
	currentLineStart := mrm.offset + 1
	currentLineEnd := mrm.offset + available

	if currentLineEnd > totalLines {
		currentLineEnd = totalLines
	}

	fmt.Fprintf(b, "  Lines %d-%d of %d | %d files total\n",
		currentLineStart, currentLineEnd, totalLines, totalFiles)
	b.WriteString("  ↑/k: up | ↓/j: down | g: top | G: bottom | q: quit\n")
}
