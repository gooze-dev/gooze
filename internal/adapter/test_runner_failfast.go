package adapter

import "context"

// RunMutantTest is RunGoTest with -failfast: go test stops at the first
// failing test, which is enough to kill a mutant. The user's test flags come
// after it.
func (a *LocalTestRunnerAdapter) RunMutantTest(ctx context.Context, workDir, testFile, overlay string) (string, error) {
	return a.runGoTest(ctx, workDir, testFile, overlay, append([]string{"-failfast"}, a.flags...))
}
