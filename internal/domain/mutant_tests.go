package domain

import "context"

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
