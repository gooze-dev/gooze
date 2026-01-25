package controller

import (
	"bytes"
	"fmt"
	"sort"

	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

// SimpleUI implements UI using cobra Command's Println.
type SimpleUI struct {
	cmd *cobra.Command
}

// NewSimpleUI creates a new SimpleUI.
func NewSimpleUI(cmd *cobra.Command) *SimpleUI {
	return &SimpleUI{cmd: cmd}
}

// Start initializes the UI.
func (s *SimpleUI) Start(_ ...StartOption) error {
	return nil
}

// Close finalizes the UI.
func (s *SimpleUI) Close() {

}

// Wait blocks until the UI is closed (no-op for SimpleUI).
func (s *SimpleUI) Wait() {
	// SimpleUI doesn't block - it just prints and continues
}

// DisplayEstimation prints the estimation results or error.
func (s *SimpleUI) DisplayEstimation(mutations []m.Mutation, err error) error {
	if err != nil {
		s.printf("estimation error: %v\n", err)
		return err
	}

	info := make(map[string]fileStat)

	for _, mutation := range mutations {
		if mutation.Source.Origin == nil {
			continue
		}

		fileHash := mutation.Source.Origin.Hash
		if fileHash == "" {
			fileHash = string(mutation.Source.Origin.ShortPath)
		}

		stat := info[fileHash]
		stat.path = string(mutation.Source.Origin.ShortPath)
		stat.count++
		info[fileHash] = stat
	}

	statsList := make([]fileStat, 0, len(info))
	for _, stat := range info {
		statsList = append(statsList, stat)
	}

	sort.Slice(statsList, func(i, j int) bool {
		return statsList[i].path < statsList[j].path
	})

	var tableBuffer bytes.Buffer

	table := tablewriter.NewWriter(&tableBuffer)
	table.SetHeader([]string{"Path", "Mutations"})
	table.SetBorder(false)
	table.SetCenterSeparator("")
	table.SetColumnAlignment([]int{tablewriter.ALIGN_LEFT, tablewriter.ALIGN_CENTER})

	pathsCount := 0

	for _, stat := range statsList {
		table.Append([]string{stat.path, fmt.Sprintf("%d", stat.count)})

		pathsCount++
	}

	table.SetFooter([]string{
		fmt.Sprintf("Total Files %d", pathsCount),
		fmt.Sprintf("%d", len(mutations)),
	})

	table.Render()
	s.printf("\n%s", tableBuffer.String())

	return nil
}

// DisplayConcurencyInfo shows concurrency settings.
func (s *SimpleUI) DisplayConcurencyInfo(threads int, shardIndex int, count int) {
	s.printf("Running %d mutations with %d worker(s) (Shard %d/%d)\n", count, threads, shardIndex, count)
}

// DusplayUpcomingTestsInfo shows the number of upcoming mutations to be tested.
func (s *SimpleUI) DusplayUpcomingTestsInfo(i int) {
	s.printf("Upcoming mutations: %d\n", i)
}

// DisplayStartingTestInfo shows info about the mutation test starting.
func (s *SimpleUI) DisplayStartingTestInfo(currentMutation m.Mutation, _ int) {
	path := ""
	if currentMutation.Source.Origin != nil {
		path = string(currentMutation.Source.Origin.ShortPath)
	}

	s.printf("Starting mutation %s (%s) %s\n", currentMutation.ID[:4], currentMutation.Type.Name, path)
}

// DisplayCompletedTestInfo shows info about the mutation test completion.
func (s *SimpleUI) DisplayCompletedTestInfo(currentMutation m.Mutation, mutationResult m.Result) {
	status := unknownStatusLabel
	if results, ok := mutationResult[currentMutation.Type]; ok && len(results) > 0 {
		status = formatTestStatus(results[0].Status)
	}

	s.printf("Completed mutation %s (%s) -> %s\n", currentMutation.ID[:4], currentMutation.Type.Name, status)

	if status == formatTestStatus(m.Survived) && len(currentMutation.DiffCode) > 0 {
		path := ""
		if currentMutation.Source.Origin != nil {
			path = string(currentMutation.Source.Origin.FullPath)
		}

		if path != "" {
			s.printf("File: %s\n", path)
		}

		s.printf("%s\n", currentMutation.DiffCode)
	}
}

func (s *SimpleUI) printf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(s.cmd.OutOrStdout(), format, args...)
}

func formatTestStatus(status m.TestStatus) string {
	switch status {
	case m.Killed:
		return "killed"
	case m.Survived:
		return "survived"
	case m.Skipped:
		return "skipped"
	case m.Error:
		return "error"
	default:
		return unknownStatusLabel
	}
}

const unknownStatusLabel = "unknown"
