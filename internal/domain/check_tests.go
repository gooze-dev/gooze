package domain

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	m "gooze.dev/pkg/gooze/internal/model"
)

// failingOutputLines is how many of the last lines of a failing package's
// go test output an error shows.
const failingOutputLines = 10

// TestChecker is a Workspace that can check a package's tests without any
// mutation. Both of gooze's workspaces are; a run checks each package once,
// before testing its mutants, with the workspace that tests them.
type TestChecker interface {
	// CheckTests runs the tests of mutation's package without any mutation,
	// in the same way Run tests a mutant. It returns a *FailingTestsError when
	// they fail, since a failing test can't tell a mutant from the original.
	CheckTests(ctx context.Context, mutation m.Mutation) error
}

// FailingTestsError reports a package whose tests fail without any mutation.
// Every mutant of it would count as killed, so none of them is tested.
type FailingTestsError struct {
	// Package is the package's directory, as go test names it.
	Package string
	// Output is what go test printed.
	Output string
	// TimedOut is set when the tests didn't finish within the mutation
	// timeout.
	TimedOut bool
}

func (e *FailingTestsError) Error() string {
	if e.TimedOut {
		return fmt.Sprintf("tests of %s don't finish within the mutation timeout without any mutation", e.Package)
	}

	return fmt.Sprintf("tests of %s fail without any mutation", e.Package)
}

// testChecks runs each package's tests once without any mutation, before
// the first of its mutants is tested, and shares the outcome between workers.
type testChecks struct {
	mu   sync.Mutex
	pkgs map[string]*testCheck
}

type testCheck struct {
	once sync.Once
	err  error
}

func newTestChecks() *testChecks {
	return &testChecks{pkgs: map[string]*testCheck{}}
}

// check returns the error of run for the package in dir, calling run only for
// the first mutant of it; the others wait for that call.
func (c *testChecks) check(dir string, run func() error) error {
	c.mu.Lock()

	entry, ok := c.pkgs[dir]
	if !ok {
		entry = &testCheck{}
		c.pkgs[dir] = entry
	}

	c.mu.Unlock()

	entry.once.Do(func() { entry.err = run() })

	return entry.err
}

// checkTests checks that the tests of mutation's package pass without any
// mutation, under the same timeout as a mutant. A check cut short by the run
// ending passes, so the mutant reports the cancellation as before.
func checkTests(ctx context.Context, checks *testChecks, ws Workspace, mutation m.Mutation, timeout time.Duration) error {
	checker, ok := ws.(TestChecker)
	if !ok || mutation.Source.Test == nil || mutation.Source.Origin == nil {
		return nil
	}

	return checks.check(filepath.Dir(mutation.Source.Origin.FullPath), func() error {
		checkCtx := ctx

		var cancel context.CancelFunc
		if timeout > 0 {
			checkCtx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		if err := checker.CheckTests(checkCtx, mutation); err != nil && ctx.Err() == nil {
			return err
		}

		return nil
	})
}

// untestedPackage is a package whose tests fail without any mutation, and how
// many of its mutants were not tested because of that.
type untestedPackage struct {
	err     *FailingTestsError
	mutants int
}

// untestedPackages collects the packages whose tests fail, in the order they
// are first seen.
type untestedPackages struct {
	order []string
	byPkg map[string]*untestedPackage
}

func (u *untestedPackages) add(err *FailingTestsError) {
	if u.byPkg == nil {
		u.byPkg = map[string]*untestedPackage{}
	}

	pkg, ok := u.byPkg[err.Package]
	if !ok {
		pkg = &untestedPackage{err: err}
		u.byPkg[err.Package] = pkg
		u.order = append(u.order, err.Package)
	}

	pkg.mutants++
}

// err returns the error that ends a run with packages whose tests fail, or
// nil when there are none.
func (u *untestedPackages) err() error {
	if len(u.order) == 0 {
		return nil
	}

	mutants := 0
	for _, pkg := range u.byPkg {
		mutants += pkg.mutants
	}

	var b strings.Builder

	fmt.Fprintf(&b, "tests fail without any mutation in %s, so %s were not tested:",
		plural(len(u.order), "package", "packages"), plural(mutants, "mutant", "mutants"))

	for _, name := range u.order {
		pkg := u.byPkg[name]
		fmt.Fprintf(&b, "\n\n%s (%s)", pkg.err.Error(), plural(pkg.mutants, "mutant", "mutants"))

		for _, line := range lastLines(pkg.err.Output, failingOutputLines) {
			b.WriteString("\n    " + line)
		}
	}

	b.WriteString("\n\nFix these tests (go test should pass), then run gooze again.")

	return WithExitCode(m.ExitTestsFail, fmt.Errorf("%s", b.String()))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}

	return fmt.Sprintf("%d %s", n, many)
}

// lastLines returns the last n non-blank lines of output.
func lastLines(output string, n int) []string {
	var kept []string

	for line := range strings.SplitSeq(output, "\n") {
		if strings.TrimSpace(line) != "" {
			kept = append(kept, line)
		}
	}

	return kept[max(len(kept)-n, 0):]
}
