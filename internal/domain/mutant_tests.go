package domain

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// MutantTestRunner is a TestRunnerAdapter that can stop the tests of a mutant
// at the first failing test: one failure kills the mutant, so the rest of the
// tests tell nothing more.
type MutantTestRunner interface {
	TestRunnerAdapter
	// RunMutantTest is RunGoTest that stops at the first failing test.
	RunMutantTest(ctx context.Context, workDir, testFile, overlay string) (output string, err error)
}

// runMutantTests runs the tests of a mutant, up to the first failing test
// when the runner can stop there.
func runMutantTests(ctx context.Context, testAdapter TestRunnerAdapter, workDir, testFile, overlay string) (string, error) {
	if runner, ok := testAdapter.(MutantTestRunner); ok {
		return runner.RunMutantTest(ctx, workDir, testFile, overlay)
	}

	return testAdapter.RunGoTest(ctx, workDir, testFile, overlay)
}

// testStatus runs the tests of testPath's package in workDir, reading the
// files mapped in overlay (if any) instead of their originals, and classifies
// the outcome. For a killed mutant it also names the first failing test, or
// "" when the output names none.
func testStatus(ctx context.Context, testAdapter TestRunnerAdapter, workDir, testPath, overlay m.Path) (rpt.TestStatus, string) {
	if err := ctx.Err(); err != nil {
		return rpt.Timeout, ""
	}

	output, testErr := runMutantTests(ctx, testAdapter, string(workDir), string(testPath), string(overlay))
	if testErr != nil {
		if ctx.Err() != nil || errors.Is(testErr, ErrMemoryLimit) || errors.Is(testErr, ErrDiskLimit) {
			return rpt.Timeout, ""
		}

		if isBuildFailure(output) {
			slog.Debug("Mutation did not compile", "test", testPath)
			return rpt.Error, ""
		}

		return rpt.Killed, killingTest(output)
	}

	if strings.Contains(output, "[no test files]") {
		return rpt.NotCovered, ""
	}

	return rpt.Survived, ""
}

// failPrefix starts the line go test -v writes for a failing test, example
// or benchmark; the lines of its subtests are indented.
const failPrefix = "--- FAIL: "

// killingTest returns the name of the first failing test in the output of go
// test -v, or "" when it names none, as when the test binary panics.
func killingTest(output string) string {
	for line := range strings.Lines(output) {
		if name, ok := strings.CutPrefix(line, failPrefix); ok {
			name, _, _ = strings.Cut(strings.TrimSpace(name), " ")
			return name
		}
	}

	return ""
}

// resultForOutcome is resultForStatus that names the test that killed the
// mutation.
func resultForOutcome(mutation m.Mutation, status rpt.TestStatus, killedBy string) m.Result {
	result := resultForStatus(mutation, status)
	result[mutation.Type][0].KilledBy = killedBy

	return result
}
