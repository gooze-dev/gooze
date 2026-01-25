package controller

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/spf13/cobra"
)

func TestSimpleUI_DisplayEstimation_PrintsTable(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	ui := NewSimpleUI(cmd)

	mutations := []m.Mutation{
		{Source: m.Source{Origin: &m.File{ShortPath: "a.go", FullPath: "path/a.go"}}},
		{Source: m.Source{Origin: &m.File{ShortPath: "a.go", FullPath: "path/a.go"}}},
		{Source: m.Source{Origin: &m.File{ShortPath: "b.go", FullPath: "path/b.go"}}},
		{Source: m.Source{Origin: nil}},
	}

	if err := ui.DisplayEstimation(mutations, nil); err != nil {
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

	if err := ui.DisplayEstimation(nil, boom); err == nil {
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
	if err := ui.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	ui.Wait()
	ui.Close()
	ui.Wait()
	ui.Close()

	ui.DisplayConcurencyInfo(3, 1, 2)
	ui.DusplayUpcomingTestsInfo(7)
	ui.DisplayStartingTestInfo(m.Mutation{ID: "abcd1234567890", Type: m.MutationArithmetic}, 0)
	ui.DisplayStartingTestInfo(m.Mutation{ID: "efgh5678901234", Type: m.MutationBoolean, Source: m.Source{Origin: &m.File{ShortPath: "a.go", FullPath: "path/a.go"}}}, 0)

	result := m.Result{
		m.MutationArithmetic: []struct {
			MutationID string
			Status     m.TestStatus
			Err        error
		}{{MutationID: "abcd1234567890", Status: m.Killed}},
		m.MutationBoolean: []struct {
			MutationID string
			Status     m.TestStatus
			Err        error
		}{{MutationID: "efgh5678901234", Status: m.Survived}},
	}
	ui.DisplayCompletedTestInfo(m.Mutation{ID: "abcd1234567890", Type: m.MutationArithmetic}, result)
	ui.DisplayCompletedTestInfo(m.Mutation{ID: "efgh5678901234", Type: m.MutationBoolean, Source: m.Source{Origin: &m.File{FullPath: "path/a.go"}}, DiffCode: []byte("--- original\n+++ mutated\n@@\n")}, result)

	output := buf.String()
	for _, want := range []string{
		"Running 2 mutations",
		"Upcoming mutations: 7",
		"Starting mutation abcd (arithmetic)",
		"Starting mutation efgh (boolean) a.go",
		"Completed mutation abcd (arithmetic) -> killed",
		"Completed mutation efgh (boolean) -> survived",
		"File: path/a.go",
		"--- original",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q\noutput:\n%s", want, output)
		}
	}
}

func TestFormatTestStatus(t *testing.T) {
	cases := map[m.TestStatus]string{
		m.Killed:         "killed",
		m.Survived:       "survived",
		m.Skipped:        "skipped",
		m.Error:          "error",
		m.TestStatus(99): unknownStatusLabel,
	}

	for status, want := range cases {
		if got := formatTestStatus(status); got != want {
			t.Fatalf("formatTestStatus(%v) = %q, want %q", status, got, want)
		}
	}
}
