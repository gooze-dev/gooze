package controller

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestSimpleUI_DisplayEstimation_PrintsTable(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	ui := NewSimpleUI(cmd)

	estimation := domain.Estimation{
		Total: 4,
		Files: []domain.FileEstimate{
			{Path: "a.go", Count: 2},
			{Path: "b.go", Count: 1},
		},
	}

	if err := ui.DisplayEstimation(context.Background(), estimation, nil); err != nil {
		t.Fatalf("DisplayEstimation() error = %v", err)
	}

	output := buf.String()

	for _, want := range []string{
		"a.go",
		"b.go",
		"2",
		"1",
		"TOTAL FILES 2",
		"4",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q\noutput:\n%s", want, output)
		}
	}
}

func TestSimpleUI_DisplayEstimation_Error(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	ui := NewSimpleUI(cmd)
	boom := errors.New("boom")

	if err := ui.DisplayEstimation(context.Background(), domain.Estimation{}, boom); err == nil {
		t.Fatalf("DisplayEstimation() expected error")
	}

	output := buf.String()
	if !strings.Contains(output, "estimation error: boom") {
		t.Fatalf("output missing error message\noutput:\n%s", output)
	}
}

func TestSimpleUI_OtherDisplays(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	ui := NewSimpleUI(cmd)
	ctx := context.Background()
	if err := ui.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	ui.Wait(ctx)
	ui.Close(ctx)
	ui.Wait(ctx)
	ui.Close(ctx)

	ui.DisplayConcurrencyInfo(ctx, 3, 1, 2)
	ui.DisplayUpcomingTestsInfo(ctx, 7)
	ui.DisplayStartingTestInfo(ctx, m.Mutation{ID: "abcd1234567890", Type: mgen.Arithmetic}, 0)
	ui.DisplayStartingTestInfo(ctx, m.Mutation{ID: "efgh5678901234", Type: mgen.Boolean, Source: rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "path/a.go"}}}, 0)

	result := m.Result{
		mgen.Arithmetic: []struct {
			MutationID string
			Status     rpt.TestStatus
			Err        error
		}{{MutationID: "abcd1234567890", Status: rpt.Killed}},
		mgen.Boolean: []struct {
			MutationID string
			Status     rpt.TestStatus
			Err        error
		}{{MutationID: "efgh5678901234", Status: rpt.Survived}},
	}
	ui.DisplayCompletedTestInfo(ctx, m.Mutation{ID: "abcd1234567890", Type: mgen.Arithmetic}, result)
	ui.DisplayCompletedTestInfo(ctx, m.Mutation{ID: "efgh5678901234", Type: mgen.Boolean, Source: rpt.Source{Origin: &rpt.File{FullPath: "path/a.go"}}, DiffCode: []byte("--- original\n+++ mutated\n@@\n")}, result)
	ui.DisplayMutationScore(ctx, 0.75)

	output := buf.String()
	for _, want := range []string{
		"Running with 3 worker(s) (shard 1/2)",
		"Upcoming mutations: 7",
		"Starting mutation abcd (arithmetic)",
		"Starting mutation efgh (boolean) a.go",
		"Completed mutation abcd (arithmetic) -> killed",
		"Completed mutation efgh (boolean) -> survived",
		"File: path/a.go",
		"--- original",
		"Mutation score: 75.00%",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q\noutput:\n%s", want, output)
		}
	}
}

func TestFormatTestStatus(t *testing.T) {
	cases := map[rpt.TestStatus]string{
		rpt.Killed:         "killed",
		rpt.Survived:       "survived",
		rpt.Skipped:        "skipped",
		rpt.Error:          "error",
		rpt.TestStatus(99): unknownStatusLabel,
	}

	for status, want := range cases {
		if got := formatTestStatus(status); got != want {
			t.Fatalf("formatTestStatus(%v) = %q, want %q", status, got, want)
		}
	}
}

func TestReportedDiff(t *testing.T) {
	diff := []byte("--- original\n+++ mutated\n")

	cases := []struct {
		name   string
		status string
		diff   []byte
		want   []byte
	}{
		{name: "killed hides diff", status: formatTestStatus(rpt.Killed), diff: diff, want: nil},
		{name: "survived shows diff", status: formatTestStatus(rpt.Survived), diff: diff, want: diff},
		{name: "error shows diff", status: formatTestStatus(rpt.Error), diff: diff, want: diff},
		{name: "empty diff", status: formatTestStatus(rpt.Survived), diff: nil, want: nil},
		{name: "zero-length diff", status: formatTestStatus(rpt.Survived), diff: []byte{}, want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reportedDiff(tc.status, tc.diff); !bytes.Equal(got, tc.want) || (tc.want == nil) != (got == nil) {
				t.Fatalf("reportedDiff(%q) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}
