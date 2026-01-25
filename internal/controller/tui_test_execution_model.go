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
		return lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("6")).
				Bold(true).
				Width(6).
				Align(lipgloss.Left),
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("6")).
				Bold(true).
				Width(10).
				Align(lipgloss.Left),
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("6")).
				Bold(true).
				Width(12).
				Align(lipgloss.Left),
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("6")).
				Bold(true),
			animateScrollFile(result.file, fileWidth, d.offset)
	}

	statusColorMap := map[string]lipgloss.Color{
		"killed":   lipgloss.Color("2"), // Green
		"survived": lipgloss.Color("1"), // Red
		"error":    lipgloss.Color("1"), // Red
		"unknown":  lipgloss.Color("8"), // Gray
	}

	statusColor, ok := statusColorMap[result.status]
	if !ok {
		statusColor = lipgloss.Color("8")
	}

	return lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true).
			Width(6).
			Align(lipgloss.Left),
		lipgloss.NewStyle().
			Foreground(statusColor).
			Bold(true).
			Width(10).
			Align(lipgloss.Left),
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("5")).
			Width(12).
			Align(lipgloss.Left),
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("14")),
		truncateFile(result.file, fileWidth)
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
	totalMutations    int
	completedCount    int
	progressPercent   float64
	threads           int
	shardIndex        int
	totalShards       int
	threadFiles       map[int]string // Maps thread ID to current file being tested
	threadMutationIDs map[int]string // Maps thread ID to current mutation ID
	rendered          bool
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
		m.threads = msg.threads
		m.shardIndex = msg.shardIndex
		m.totalShards = msg.shards
		m.progressPercent = 0

	case upcomingMsg:
		m.totalMutations = msg.count
		m.completedCount = 0
		m.progressPercent = 0

	case estimationMsg:
		// Shouldn't happen in test execution model, but handle gracefully
	}

	return m, cmd
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

	// 1. Title
	title := titleStyle.Render("🧬 Gooze Mutation Testing")

	// 2. Summary with metadata
	summary := summaryStyle.Render(fmt.Sprintf(
		"Progress: %s / %s  •  Threads: %s  •  Shard: %s / %s",
		accentStyle.Render(fmt.Sprintf("%d", m.completedCount)),
		accentStyle.Render(fmt.Sprintf("%d", m.totalMutations)),
		accentStyle.Render(fmt.Sprintf("%d", m.threads)),
		accentStyle.Render(fmt.Sprintf("%d", m.shardIndex)),
		accentStyle.Render(fmt.Sprintf("%d", m.totalShards)),
	))

	// 3. Progress Bar
	progressStyle := lipgloss.NewStyle().
		Padding(0, 2)

	progressView := progressStyle.Render(m.progressBar.ViewAs(m.progressPercent))

	// 4. Thread Progress Section
	threadsBox := m.renderThreadBox(accentColor)

	// 5. Footer
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Align(lipgloss.Center).
		Width(m.width).
		Padding(0, 0)

	footer := footerStyle.Render("Press q to quit")

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		summary,
		progressView,
		threadsBox,
		footer,
	)
}

func (m testExecutionModel) renderThreadBox(accentColor lipgloss.Color) string {
	contentStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(0, 1). // Compact padding
		Margin(1, 1, 1, 0).
		Width(m.width - 4) // Constrain width

	fileStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14"))

	threadLines := make([]string, 0, m.threads)
	// Calculate max specific width for file path:
	// Width - Border(2) - Padding(2)
	availableWidth := m.width - 4 - 2 - 2
	prefixWidth := 0
	threadLabelFormat := ""

	if m.threads > 1 {
		// Calculate width needed for thread number
		digits := len(fmt.Sprintf("%d", m.threads-1))
		prefixWidth = 7 + digits + 2 // "Thread " + digits + ": "
		threadLabelFormat = fmt.Sprintf("Thread %%%dd: %%s", digits)
	}

	for i := range m.threads {
		file := m.threadFiles[i]
		mutationID := m.threadMutationIDs[i]

		var lineContent string

		if file == "" {
			lineContent = "idle"
		} else {
			// Construct ID string
			idStr := ""
			if mutationID != "" {
				idStr = fmt.Sprintf("ID: %-4s ", mutationID[:4])
			} // Calculate remaining width for file path
			// available - prefix - id length
			remainingForFile := availableWidth - prefixWidth - len(idStr)
			if remainingForFile < 10 {
				remainingForFile = 10
			}

			truncatedFile := truncateFile(file, remainingForFile)
			lineContent = fmt.Sprintf("%s%s",
				lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(idStr), // Grey for ID
				fileStyle.Render(truncatedFile),
			)
		}

		var threadLine string
		if m.threads > 1 {
			threadLine = fmt.Sprintf(threadLabelFormat,
				i,
				lineContent,
			)
		} else {
			threadLine = lineContent
		}

		threadLines = append(threadLines, threadLine)
	}

	// Join lines and put in one box
	threadsContent := lipgloss.JoinVertical(lipgloss.Left, threadLines...)

	return contentStyle.Render(threadsContent)
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
	summary := summaryStyle.Render(fmt.Sprintf(
		"Total: %s  •  Killed: %s  •  Survived: %s  •  Errors: %s",
		accentStyle.Render(fmt.Sprintf("%d", len(m.results))),
		accentStyle.Render(fmt.Sprintf("%d", m.countStatus("killed"))),
		accentStyle.Render(fmt.Sprintf("%d", m.countStatus("survived"))),
		accentStyle.Render(fmt.Sprintf("%d", m.countStatus("error"))),
	))

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

func animateScrollFile(text string, width int, offset int) string {
	if width <= 0 {
		return ""
	}

	textWidth := lipgloss.Width(text)
	if textWidth <= width {
		return text
	}

	// Gap between repeats
	gap := "   "

	// Initial pause before scrolling starts (in ticks)
	pause := 5

	if offset < pause {
		return truncateFile(text, width)
	}

	effectiveStep := offset - pause

	// Create the repeating pattern: text + gap
	runes := []rune(text + gap)
	n := len(runes)

	if n == 0 {
		return ""
	}

	start := effectiveStep % n

	// Construct the window
	res := make([]rune, 0, width)
	for i := range width {
		idx := (start + i) % n
		res = append(res, runes[idx])
	}

	return string(res)
}

func truncateFile(text string, width int) string {
	if width <= 0 {
		return ""
	}

	if lipgloss.Width(text) <= width {
		return text
	}

	if width <= 1 {
		return "…"
	}

	ellipsis := "…"

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

	lines := strings.Split(diff, "\n")
	maxLines := m.diffMaxLines()
	truncated := false

	if len(lines) > maxLines {
		lines = lines[:maxLines-1]
		truncated = true
	}

	contentWidth := width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	bodyLines := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		bodyLines = append(bodyLines, renderDiffLine(line, contentWidth))
	}

	if truncated {
		bodyLines = append(bodyLines, truncateFile("…", contentWidth))
	}

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Bold(true)

	headerText := "Diff"
	if m.selectedDiffPath != "" {
		headerText = fmt.Sprintf("Diff • %s", m.selectedDiffPath)
	}

	header := headerStyle.Render(truncateFile(headerText, contentWidth))

	body := lipgloss.JoinVertical(lipgloss.Left, bodyLines...)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Margin(0, 1, 0, 0).
		Padding(0, 1).
		Width(width)

	box := boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body))

	return box, lipgloss.Height(box)
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

	return style.Render(truncateFile(line, width))
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
