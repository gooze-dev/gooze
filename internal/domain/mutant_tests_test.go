package domain

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// plainRunner can only run every test.
type plainRunner struct{ calls []string }

func (r *plainRunner) RunGoTest(_ context.Context, workDir, testFile, overlay string) (string, error) {
	r.calls = append(r.calls, "all "+workDir+" "+testFile+" "+overlay)

	return "--- FAIL: TestA\n", errors.New("exit status 1")
}

// failFastRunner can also stop at the first failing test.
type failFastRunner struct{ plainRunner }

func (r *failFastRunner) RunMutantTest(_ context.Context, workDir, testFile, overlay string) (string, error) {
	r.calls = append(r.calls, "failfast "+workDir+" "+testFile+" "+overlay)

	return "ok\n", nil
}

func TestTestStatus_StopsAMutantsTestsAtTheFirstFailure(t *testing.T) {
	runner := &failFastRunner{}

	status := testStatus(context.Background(), runner, "/w", "/w/p/a.go", "/tmp/overlay.json")

	assert.Equal(t, rpt.Survived, status, "the outcome is read from the mutant run")
	assert.Equal(t, []string{"failfast /w /w/p/a.go /tmp/overlay.json"}, runner.calls)
}

func TestTestStatus_RunsEveryTestWithARunnerThatCantStop(t *testing.T) {
	runner := &plainRunner{}

	status := testStatus(context.Background(), runner, "/w", "/w/p/a.go", "")

	assert.Equal(t, rpt.Killed, status)
	assert.Equal(t, []string{"all /w /w/p/a.go "}, runner.calls)
}

func TestRunUnmutatedTests_RunsEveryTest(t *testing.T) {
	runner := &failFastRunner{}
	mutation := m.Mutation{Source: rpt.Source{Origin: &rpt.File{ShortPath: "p/a.go"}}}

	err := runUnmutatedTests(context.Background(), runner, "/w", "/w/p/a_test.go", mutation)

	var failing *FailingTestsError

	require.ErrorAs(t, err, &failing)
	assert.Equal(t, "--- FAIL: TestA\n", failing.Output, "the baseline shows every failing test")
	assert.Equal(t, []string{"all /w /w/p/a_test.go "}, runner.calls)
}
