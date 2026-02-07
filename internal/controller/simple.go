package controller

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	m "gooze.dev/pkg/gooze/internal/model"
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
func (s *SimpleUI) Start(ctx context.Context, _ ...StartOption) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return nil
}

// Close finalizes the UI.
func (s *SimpleUI) Close(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		return
	}
}

// Wait blocks until the UI is closed (no-op for SimpleUI).
func (s *SimpleUI) Wait(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		return
	}
	// SimpleUI doesn't block - it just prints and continues
}

// DisplayEstimation prints the estimation results or error.
func (s *SimpleUI) DisplayEstimation(ctx context.Context, mutations []m.Mutation, err error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err != nil {
		s.printf("estimation error: %v\n", err)
		return err
	}

	statsList := buildFileStats(mutations)
	tableStr := renderEstimationTable(statsList, len(mutations))
	s.printf("\n%s", tableStr)

	return nil
}

func buildFileStats(mutations []m.Mutation) []fileStat {
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

	return statsList
}

func renderEstimationTable(statsList []fileStat, totalMutations int) string {
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
		fmt.Sprintf("%d", totalMutations),
	})

	table.Render()

	return tableBuffer.String()
}

// DisplayConcurrencyInfo shows concurrency settings.
func (s *SimpleUI) DisplayConcurrencyInfo(ctx context.Context, threads int, shardIndex int, count int) {
	if err := ctx.Err(); err != nil {
		return
	}

	s.printf("Running %d mutations with %d worker(s) (Shard %d/%d)\n", count, threads, shardIndex, count)
}

// DisplayUpcomingTestsInfo shows the number of upcoming mutations to be tested.
func (s *SimpleUI) DisplayUpcomingTestsInfo(ctx context.Context, i int) {
	if err := ctx.Err(); err != nil {
		return
	}

	s.printf("Upcoming mutations: %d\n", i)
}

// DisplayStartingTestInfo shows info about the mutation test starting.
func (s *SimpleUI) DisplayStartingTestInfo(ctx context.Context, currentMutation m.Mutation, _ int) {
	if err := ctx.Err(); err != nil {
		return
	}

	path := ""
	if currentMutation.Source.Origin != nil {
		path = string(currentMutation.Source.Origin.ShortPath)
	}

	s.printf("Starting mutation %s (%s) %s\n", currentMutation.ID[:4], currentMutation.Type.Name, path)
}

// DisplayCompletedTestInfo shows info about the mutation test completion.
func (s *SimpleUI) DisplayCompletedTestInfo(ctx context.Context, currentMutation m.Mutation, mutationResult m.Result) {
	if err := ctx.Err(); err != nil {
		return
	}

	status := unknownStatusLabel
	if results, ok := mutationResult[currentMutation.Type]; ok && len(results) > 0 {
		status = formatTestStatus(results[0].Status)
	}

	s.printf("Completed mutation %s (%s) -> %s\n", currentMutation.ID[:4], currentMutation.Type.Name, status)

	if status != formatTestStatus(m.Killed) && len(currentMutation.DiffCode) > 0 {
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

// DisplayMutationScore prints the final mutation score.
func (s *SimpleUI) DisplayMutationScore(ctx context.Context, score float64) {
	if err := ctx.Err(); err != nil {
		return
	}

	s.printf("Mutation score: %.2f%%\n", score*100)
}

func (s *SimpleUI) printf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(s.cmd.OutOrStdout(), format, args...)
}

func formatTestStatus(status m.TestStatus) string {
	return status.String()
}

const unknownStatusLabel = "unknown"
