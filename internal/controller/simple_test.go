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
		{Source: m.Source{Origin: &m.File{Path: "path/a.go"}}},
		{Source: m.Source{Origin: &m.File{Path: "path/a.go"}}},
		{Source: m.Source{Origin: &m.File{Path: "path/b.go"}}},
		{Source: m.Source{Origin: nil}},
	}

	if err := ui.DisplayEstimation(mutations, nil); err != nil {
		t.Fatalf("DisplayEstimation() error = %v", err)
	}

	output := buf.String()

	for _, want := range []string{
		"path/a.go",
		"path/b.go",
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
	ui.DisplayStartingTestInfo(m.Mutation{ID: 10, Type: m.MutationArithmetic}, 0)
	ui.DisplayStartingTestInfo(m.Mutation{ID: 11, Type: m.MutationBoolean, Source: m.Source{Origin: &m.File{Path: "path/a.go"}}}, 0)

	result := m.Result{
		m.MutationArithmetic: []struct {
			MutationID string
			Status     m.TestStatus
			Err        error
		}{{MutationID: "10", Status: m.Killed}},
	}
	ui.DisplayCompletedTestInfo(m.Mutation{ID: 10, Type: m.MutationArithmetic}, result)
	ui.DisplayCompletedTestInfo(m.Mutation{ID: 11, Type: m.MutationBoolean}, m.Result{})

	output := buf.String()
	for _, want := range []string{
		"Running 2 mutations",
		"Upcoming mutations: 7",
		"Starting mutation 10 (arithmetic)",
		"Starting mutation 11 (boolean) path/a.go",
		"Completed mutation 10 (arithmetic) -> killed",
		"Completed mutation 11 (boolean) -> unknown",
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
