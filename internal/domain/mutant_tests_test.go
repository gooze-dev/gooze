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

	status, killedBy := testStatus(context.Background(), runner, "/w", "/w/p/a.go", "/tmp/overlay.json")

	assert.Equal(t, rpt.Survived, status, "the outcome is read from the mutant run")
	assert.Empty(t, killedBy, "no test kills a survivor")
	assert.Equal(t, []string{"failfast /w /w/p/a.go /tmp/overlay.json"}, runner.calls)
}

func TestTestStatus_RunsEveryTestWithARunnerThatCantStop(t *testing.T) {
	runner := &plainRunner{}

	status, killedBy := testStatus(context.Background(), runner, "/w", "/w/p/a.go", "")

	assert.Equal(t, rpt.Killed, status)
	assert.Equal(t, "TestA", killedBy)
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

func TestKillingTest(t *testing.T) {
	for name, tt := range map[string]struct{ output, want string }{
		"one failing test": {
			output: "=== RUN   TestA\n--- PASS: TestA (0.00s)\n=== RUN   TestB\n    b_test.go:9: boom\n--- FAIL: TestB (0.01s)\nFAIL\nFAIL\texample.com/p\t0.01s\n",
			want:   "TestB",
		},
		"the first of several": {output: "--- FAIL: TestB (0.00s)\n--- FAIL: TestC (0.00s)\nFAIL\n", want: "TestB"},
		"a subtest names its test": {
			output: "=== RUN   TestTable\n=== RUN   TestTable/zero\n--- FAIL: TestTable (0.00s)\n    --- FAIL: TestTable/zero (0.00s)\nFAIL\n",
			want:   "TestTable",
		},
		"a failing example":         {output: "--- FAIL: ExampleSum (0.00s)\ngot:\n3\nwant:\n4\nFAIL\n", want: "ExampleSum"},
		"without a duration":        {output: "--- FAIL: TestA\n", want: "TestA"},
		"at the end of the output":  {output: "ok\n--- FAIL: TestA", want: "TestA"},
		"no failing test is named":  {output: "panic: boom\nFAIL\texample.com/p\t0.01s\n", want: ""},
		"a test log that quotes it": {output: "    a_test.go:3: want --- FAIL: TestX\nFAIL\n", want: ""},
		"only passes":               {output: "--- PASS: TestA (0.00s)\nPASS\n", want: ""},
		"no output":                 {output: "", want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, killingTest(tt.output))
		})
	}
}

func TestResultForOutcome_NamesTheKillingTest(t *testing.T) {
	mutation := m.Mutation{ID: "abcd"}

	killed := resultForOutcome(mutation, rpt.Killed, "TestA")
	assert.Equal(t, m.Result{mutation.Type: {{MutationID: "abcd", Status: rpt.Killed, KilledBy: "TestA"}}}, killed)

	survived := resultForOutcome(mutation, rpt.Survived, "")
	assert.Equal(t, m.Result{mutation.Type: {{MutationID: "abcd", Status: rpt.Survived}}}, survived)
}
