package controller

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// testResult holds information about a completed mutation test.
type testResult struct {
	id     string
	file   string
	typ    string
	status string
	diff   string
}

// Implement list.Item interface for testResult.
func (r testResult) FilterValue() string {
	return r.id + " " + r.file + " " + r.typ + " " + r.status
}

// testResultDelegate is the delegate for rendering test results in the list.
type testResultDelegate struct {
	offset int
}

func (d testResultDelegate) Height() int  { return 1 }
func (d testResultDelegate) Spacing() int { return 0 }
func (d testResultDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d testResultDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	result, ok := item.(testResult)
	if !ok {
		return
	}

	isSelected := index == m.Index()
	fileWidth := m.Width() - 40 // Reserve space for ID, Status, Type columns and spacing

	idStyle, statusStyle, typeStyle, fileStyle, displayFile := d.getStylesAndFile(result, isSelected, fileWidth)

	line := fmt.Sprintf("%s  %s  %s  %s",
		idStyle.Render(fmt.Sprintf("%-4s", result.id[:4])),
		statusStyle.Render(fmt.Sprintf("%-8s", result.status)),
		typeStyle.Render(fmt.Sprintf("%-10s", result.typ)),
		fileStyle.Render(displayFile),
	)
	_, _ = fmt.Fprint(w, line)
}

func (d testResultDelegate) getStylesAndFile(result testResult, isSelected bool, fileWidth int) (lipgloss.Style, lipgloss.Style, lipgloss.Style, lipgloss.Style, string) {
	if isSelected {
		base := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Bold(true)

		return base.Width(6).Align(lipgloss.Left),
			base.Width(10).Align(lipgloss.Left),
			base.Width(12).Align(lipgloss.Left),
			base,
			animateScroll(result.file, fileWidth, d.offset)
	}

	idStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Width(6).Align(lipgloss.Left)
	statusStyle := lipgloss.NewStyle().Foreground(statusColor(result.status)).Bold(true).Width(10).Align(lipgloss.Left)
	typeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Width(12).Align(lipgloss.Left)
	fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14"))

	return idStyle, statusStyle, typeStyle, fileStyle, truncateToWidth(result.file, fileWidth)
}

// statusColor maps a mutation status to its display color.
func statusColor(status string) lipgloss.Color {
	switch status {
	case "killed":
		return lipgloss.Color("2") // Green
	case "survived", "error":
		return lipgloss.Color("1") // Red
	default:
		return lipgloss.Color("8") // Gray
	}
}

// testExecutionModel handles the TUI display during mutation testing.
type testExecutionModel struct {
	width             int
	height            int
	progressBar       progress.Model
	currentFile       string
	currentMutationID string
	currentType       string
	currentStatus     string
	mutationScore     float64
	mutationScoreSet  bool
	totalMutations    int
	completedCount    int
	progressPercent   float64
	threads           int
	shardIndex        int
	totalShards       int
	threadFiles       map[int]string // Maps thread ID to current file being tested
	threadMutationIDs map[int]string // Maps thread ID to current mutation ID
	rendered          bool
	counting          bool // true while the upfront count pass runs (before the total is known)
	testingFinished   bool
	results           []testResult
	resultsList       list.Model
	delegate          testResultDelegate
	animOffset        int
	lastSelected      int
	showDiff          bool
	selectedDiff      string
	selectedDiffPath  string
}

func newTestExecutionModel() testExecutionModel {
	prog := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	delegate := testResultDelegate{}
	resultsList := list.New([]list.Item{}, delegate, 80, 20)
	resultsList.SetShowPagination(false)
	resultsList.SetShowFilter(true)
	resultsList.SetShowHelp(false)
	resultsList.SetShowTitle(false)
	resultsList.SetShowStatusBar(false)
	resultsList.FilterInput.Placeholder = "Filter results…"

	return testExecutionModel{
		progressBar:       prog,
		resultsList:       resultsList,
		delegate:          delegate,
		threadFiles:       make(map[int]string),
		threadMutationIDs: make(map[int]string),
		lastSelected:      -1,
	}
}

func (m testExecutionModel) Init() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m testExecutionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.handleWindowSize(msg)

	case tea.KeyMsg:
		m, cmd = m.handleKeyMsg(msg)

	case tea.MouseMsg:
		m, cmd = m.handleMouseMsg(msg)

	case tickMsg:
		return m.handleTickMsg(msg)

	case startMutationMsg:
		m = m.handleStartMutation(msg)

	case completedMutationMsg:
		m = m.handleCompletedMutation(msg)

	case concurrencyMsg:
		m = m.handleConcurrency(msg)

	case upcomingMsg:
		m = m.handleUpcoming(msg)

	case mutationScoreMsg:
		m.mutationScore = msg.score
		m.mutationScoreSet = true
	}

	return m, cmd
}

func (m testExecutionModel) handleConcurrency(msg concurrencyMsg) testExecutionModel {
	m.threads = msg.threads
	m.shardIndex = msg.shardIndex
	m.totalShards = msg.shards
	m.progressPercent = 0
	// Concurrency info arrives before the count pass finishes, so show a counting
	// state instead of leaving the screen on "Initializing…".
	m.counting = true
	m.rendered = true

	return m
}

func (m testExecutionModel) handleUpcoming(msg upcomingMsg) testExecutionModel {
	m.totalMutations = msg.count
	m.completedCount = 0
	m.progressPercent = 0
	m.counting = false
	m.rendered = true

	if msg.count == 0 {
		m.testingFinished = true
		m.progressPercent = 1.0
	}

	return m
}

func (m testExecutionModel) View() string {
	if !m.rendered {
		return "Initializing test execution…\n"
	}

	if m.testingFinished {
		return m.viewResults()
	}

	return m.viewProgress()
}

func (m testExecutionModel) viewProgress() string {
	accentColor := lipgloss.Color("6") // Cyan

	// Styles
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true).
		Padding(1, 0, 0, 2)

	summaryStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Padding(0, 0, 1, 2)

	accentStyle := lipgloss.NewStyle().Foreground(accentColor) // Cyan

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Align(lipgloss.Center).
		Width(m.width).
		Padding(0, 0)

	footer := footerStyle.Render("Press q to quit")

	// 1. Title
	title := titleStyle.Render("🧬 Gooze Mutation Testing")

	// While the upfront count pass runs the total isn't known yet.
	if m.counting {
		return lipgloss.JoinVertical(lipgloss.Left,
			title,
			summaryStyle.Render("Counting mutations…"),
			footer,
		)
	}

	// 2. Summary with metadata
	summary := m.progressSummary(summaryStyle, accentStyle)

	// 3. Progress Bar
	progressView := lipgloss.NewStyle().Padding(0, 2).Render(m.progressBar.ViewAs(m.progressPercent))

	// 4. Thread Progress Section
	threadsBox := m.renderThreadBox(accentColor)

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		summary,
		progressView,
		threadsBox,
		footer,
	)
}

// progressSummary renders the one-line "Progress / Threads / Shard" header.
func (m testExecutionModel) progressSummary(summaryStyle, accentStyle lipgloss.Style) string {
	num := func(n int) string { return accentStyle.Render(fmt.Sprintf("%d", n)) }

	return summaryStyle.Render(fmt.Sprintf(
		"Progress: %s / %s  •  Threads: %s  •  Shard: %s / %s",
		num(m.completedCount), num(m.totalMutations), num(m.threads), num(m.shardIndex), num(m.totalShards),
	))
}

func (m testExecutionModel) renderThreadBox(accentColor lipgloss.Color) string {
	contentStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(0, 1).
		Margin(1, 1, 1, 0).
		Width(m.width - 4)

	// Width available for a thread line: box width minus border(2) and padding(2).
	fileBudget := m.width - 4 - 2 - 2
	threadLabelFormat := ""

	if m.threads > 1 {
		digits := len(fmt.Sprintf("%d", m.threads-1))
		fileBudget -= 7 + digits + 2 // "Thread " + digits + ": "
		threadLabelFormat = fmt.Sprintf("Thread %%%dd: %%s", digits)
	}

	threadLines := make([]string, 0, m.threads)

	for i := range m.threads {
		line := m.threadLineContent(i, fileBudget)
		if m.threads > 1 {
			line = fmt.Sprintf(threadLabelFormat, i, line)
		}

		threadLines = append(threadLines, line)
	}

	return contentStyle.Render(lipgloss.JoinVertical(lipgloss.Left, threadLines...))
}

// threadLineContent renders a single thread's status line: "idle", or its
// current mutation ID and (truncated) file.
func (m testExecutionModel) threadLineContent(thread, fileBudget int) string {
	file := m.threadFiles[thread]
	if file == "" {
		return "idle"
	}

	idStr := ""
	if id := m.threadMutationIDs[thread]; id != "" {
		idStr = fmt.Sprintf("ID: %-4s ", id[:4])
	}

	remainingForFile := fileBudget - len(idStr)
	if remainingForFile < 10 {
		remainingForFile = 10
	}

	return fmt.Sprintf("%s%s",
		lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(idStr), // Grey for ID
		lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Render(truncateToWidth(file, remainingForFile)),
	)
}

func (m testExecutionModel) viewResults() string {
	accentColor := lipgloss.Color("6") // Cyan

	// Styles
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true).
		Padding(1, 0, 0, 2)

	summaryStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Padding(0, 0, 1, 2)

	accentStyle := lipgloss.NewStyle().Foreground(accentColor)

	// 1. Title
	title := titleStyle.Render("🧬 Gooze Test Results")

	// 2. Summary
	summaryParts := []string{
		fmt.Sprintf("Total: %s", accentStyle.Render(fmt.Sprintf("%d", len(m.results)))),
		fmt.Sprintf("Killed: %s", accentStyle.Render(fmt.Sprintf("%d", m.countStatus("killed")))),
		fmt.Sprintf("Survived: %s", accentStyle.Render(fmt.Sprintf("%d", m.countStatus("survived")))),
		fmt.Sprintf("Errors: %s", accentStyle.Render(fmt.Sprintf("%d", m.countStatus("error")))),
	}

	if m.mutationScoreSet {
		summaryParts = append(summaryParts, fmt.Sprintf("Score: %s", accentStyle.Render(fmt.Sprintf("%.2f%%", m.mutationScore*100))))
	}

	summary := summaryStyle.Render(strings.Join(summaryParts, "  •  "))

	// 3. Results table with list
	resultsBox := m.renderResultsBox(accentColor)

	// 4. Footer
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Align(lipgloss.Center).
		Width(m.width)

	footer := footerStyle.Render("↑/k up • ↓/j down • g/G top/bottom • / filter • enter/space/click diff • q quit")

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		summary,
		resultsBox,
		footer,
	)
}

func (m testExecutionModel) renderResultsBox(accentColor lipgloss.Color) string {
	listWidth := m.width - 4
	diffBoxHeight := m.diffBoxHeight()

	listHeight := m.height - 9 - diffBoxHeight
	if listHeight < 5 {
		listHeight = 5
	}

	m.resultsList.SetHeight(listHeight)
	m.resultsList.SetWidth(listWidth)

	// Column Headers
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Bold(true).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(lipgloss.Color("8")).
		Width(listWidth)

	headers := headerStyle.Render(fmt.Sprintf("%6s  %10s  %12s  %s", "ID", "Status", "Type", "File"))

	resultsStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Margin(0, 1, 0, 0).
		Padding(0, 1)

	resultsBox := resultsStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			headers,
			m.resultsList.View(),
		),
	)

	diffBox, _ := m.renderDiffBox(accentColor, listWidth)
	if diffBox == "" {
		return resultsBox
	}

	return lipgloss.JoinVertical(lipgloss.Left, resultsBox, diffBox)
}

func (m testExecutionModel) countStatus(status string) int {
	count := 0

	for _, result := range m.results {
		if result.status == status {
			count++
		}
	}

	return count
}

func (m testExecutionModel) handleStartMutation(msg startMutationMsg) testExecutionModel {
	m.currentFile = msg.displayPath
	m.currentMutationID = msg.id
	m.currentType = fmt.Sprintf("%v", msg.kind)
	m.currentStatus = "running"
	// Track which file this thread is working on
	m.threadFiles[msg.thread] = msg.displayPath
	m.threadMutationIDs[msg.thread] = msg.id
	m.rendered = true

	return m
}

func (m testExecutionModel) handleCompletedMutation(msg completedMutationMsg) testExecutionModel {
	m.completedCount++
	m.currentStatus = msg.status

	// The worker that ran this mutation is now free; mark its thread idle until
	// it picks up the next one. (If the next mutation already started on that
	// thread, the IDs won't match and we leave it as-is.)
	for thread, id := range m.threadMutationIDs {
		if id == msg.id {
			delete(m.threadFiles, thread)
			delete(m.threadMutationIDs, thread)

			break
		}
	}

	result := testResult{
		id:     msg.id[:4],
		file:   msg.displayPath,
		typ:    fmt.Sprintf("%v", msg.kind),
		status: msg.status,
		diff:   string(msg.diff),
	}
	m.results = append(m.results, result)

	// Update results list with new items
	items := make([]list.Item, 0, len(m.results))

	for _, r := range m.results {
		items = append(items, r)
	}

	m.resultsList.SetItems(items)

	if m.totalMutations > 0 {
		m.progressPercent = float64(m.completedCount) / float64(m.totalMutations)
		// Mark as finished when all are complete
		if m.completedCount == m.totalMutations {
			m.testingFinished = true
		}
	} else if m.totalMutations == 0 {
		// If there are no mutations to test (e.g., due to caching), mark as finished immediately
		m.testingFinished = true
		m.progressPercent = 1.0
	}

	return m
}

func (m testExecutionModel) handleKeyMsg(msg tea.KeyMsg) (testExecutionModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	default:
		if m.testingFinished {
			if msg.String() == "enter" || msg.String() == " " {
				m.toggleSelectedDiff()
				return m, nil
			}

			var newList list.Model

			newList, cmd = m.resultsList.Update(msg)
			m.resultsList = newList

			// Detect selection change to reset animation
			if m.resultsList.Index() != m.lastSelected {
				m.lastSelected = m.resultsList.Index()
				m.animOffset = 0
				m.delegate.offset = 0
				m.resultsList.SetDelegate(m.delegate)
				m.showDiff = false
				m.selectedDiff = ""
				m.selectedDiffPath = ""
			}

			return m, cmd
		}
	}

	return m, nil
}

func (m testExecutionModel) handleMouseMsg(msg tea.MouseMsg) (testExecutionModel, tea.Cmd) {
	var cmd tea.Cmd

	if !m.testingFinished {
		return m, nil
	}

	var newList list.Model

	newList, cmd = m.resultsList.Update(msg)
	m.resultsList = newList

	if m.resultsList.Index() != m.lastSelected {
		m.lastSelected = m.resultsList.Index()
		m.animOffset = 0
		m.delegate.offset = 0
		m.resultsList.SetDelegate(m.delegate)
		m.showDiff = false
		m.selectedDiff = ""
		m.selectedDiffPath = ""
	}

	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionRelease && m.resultsList.FilterState() != list.Filtering {
		m.toggleSelectedDiff()
	}

	return m, cmd
}

func (m *testExecutionModel) toggleSelectedDiff() {
	item := m.resultsList.SelectedItem()

	result, ok := item.(testResult)
	if !ok {
		return
	}

	diff := strings.TrimSpace(result.diff)
	if diff == "" {
		m.showDiff = false
		m.selectedDiff = ""

		return
	}

	if m.showDiff && m.selectedDiff == diff {
		m.showDiff = false
		m.selectedDiff = ""
		m.selectedDiffPath = ""

		return
	}

	m.showDiff = true
	m.selectedDiff = diff
	m.selectedDiffPath = result.file
}

func (m testExecutionModel) diffMaxLines() int {
	maxLines := m.height / 3
	if maxLines < 6 {
		maxLines = 6
	}

	if maxLines > 20 {
		maxLines = 20
	}

	return maxLines
}

func (m testExecutionModel) diffBoxHeight() int {
	if !m.showDiff {
		return 0
	}

	diff := strings.TrimSpace(m.selectedDiff)
	if diff == "" {
		return 0
	}

	lines := strings.Split(diff, "\n")

	maxLines := m.diffMaxLines()
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	return len(lines) + 3
}

func (m testExecutionModel) renderDiffBox(accentColor lipgloss.Color, width int) (string, int) {
	if !m.showDiff {
		return "", 0
	}

	diff := strings.TrimSpace(m.selectedDiff)
	if diff == "" {
		return "", 0
	}

	contentWidth := width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	header := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true).
		Render(truncateToWidth(m.diffHeader(), contentWidth))

	body := lipgloss.JoinVertical(lipgloss.Left, m.diffBodyLines(diff, contentWidth)...)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Margin(0, 1, 0, 0).
		Padding(0, 1).
		Width(width).
		Render(lipgloss.JoinVertical(lipgloss.Left, header, body))

	return box, lipgloss.Height(box)
}

func (m testExecutionModel) diffHeader() string {
	if m.selectedDiffPath != "" {
		return fmt.Sprintf("Diff • %s", m.selectedDiffPath)
	}

	return "Diff"
}

// diffBodyLines renders the diff lines, capped to the available height with a
// trailing ellipsis when truncated.
func (m testExecutionModel) diffBodyLines(diff string, contentWidth int) []string {
	lines := strings.Split(diff, "\n")

	maxLines := m.diffMaxLines()
	truncated := false

	if len(lines) > maxLines {
		lines = lines[:maxLines-1]
		truncated = true
	}

	bodyLines := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		bodyLines = append(bodyLines, renderDiffLine(line, contentWidth))
	}

	if truncated {
		bodyLines = append(bodyLines, truncateToWidth("…", contentWidth))
	}

	return bodyLines
}

func renderDiffLine(line string, width int) string {
	trimmed := strings.TrimSpace(line)

	style := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	switch {
	case strings.HasPrefix(line, "+++"):
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	case strings.HasPrefix(line, "---"):
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	case strings.HasPrefix(line, "@@"):
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	case strings.HasPrefix(line, "+"):
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	case strings.HasPrefix(line, "-"):
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	case trimmed == "":
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	}

	return style.Render(truncateToWidth(line, width))
}

func (m testExecutionModel) handleWindowSize(msg tea.WindowSizeMsg) testExecutionModel {
	m.width = msg.Width
	m.height = msg.Height

	m.progressBar.Width = m.width - 8
	if m.progressBar.Width < 20 {
		m.progressBar.Width = 20
	}

	return m
}

func (m testExecutionModel) handleTickMsg(_ tickMsg) (testExecutionModel, tea.Cmd) {
	// Keep the UI responsive
	if m.testingFinished && m.resultsList.FilterState() != list.Filtering {
		m.animOffset++
		m.delegate.offset = m.animOffset
		m.resultsList.SetDelegate(m.delegate)
	}

	return m, tea.Tick(time.Millisecond*150, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
