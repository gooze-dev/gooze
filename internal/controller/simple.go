package controller

import (
	"bytes"
	"context"
	"fmt"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"gooze.dev/pkg/gooze/internal/domain"
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

// StartEstimate initializes the UI in estimation mode.
func (s *SimpleUI) StartEstimate(ctx context.Context) error {
	return s.Start(ctx, WithEstimateMode())
}

// StartTest initializes the UI in test execution mode.
func (s *SimpleUI) StartTest(ctx context.Context) error {
	return s.Start(ctx, WithTestMode())
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
func (s *SimpleUI) DisplayEstimation(ctx context.Context, estimation domain.Estimation, err error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err != nil {
		s.printf("estimation error: %v\n", err)
		return err
	}

	tableStr := renderEstimationTable(estimation)
	s.printf("\n%s", tableStr)

	return nil
}

func renderEstimationTable(estimation domain.Estimation) string {
	var tableBuffer bytes.Buffer

	table := tablewriter.NewWriter(&tableBuffer)
	table.SetHeader([]string{"Path", "Mutations"})
	table.SetBorder(false)
	table.SetCenterSeparator("")
	table.SetColumnAlignment([]int{tablewriter.ALIGN_LEFT, tablewriter.ALIGN_CENTER})

	for _, file := range estimation.Files {
		table.Append([]string{file.Path, fmt.Sprintf("%d", file.Count)})
	}

	table.SetFooter([]string{
		fmt.Sprintf("Total Files %d", len(estimation.Files)),
		fmt.Sprintf("%d", estimation.Total),
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
