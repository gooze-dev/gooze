package domain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLastLines(t *testing.T) {
	output := "a\n\n  \nb\nc\n"

	assert.Equal(t, []string{"b", "c"}, lastLines(output, 2))
	assert.Equal(t, []string{"a", "b", "c"}, lastLines(output, 10))
	assert.Equal(t, []string{"4", "5"}, lastLines("1\n2\n3\n4\n5", 2))
	assert.Empty(t, lastLines("\n \n", 3))
}

func TestPlural(t *testing.T) {
	assert.Equal(t, "0 mutants", plural(0, "mutant", "mutants"))
	assert.Equal(t, "1 mutant", plural(1, "mutant", "mutants"))
	assert.Equal(t, "2 mutants", plural(2, "mutant", "mutants"))
}

func TestPackageOf(t *testing.T) {
	assert.Equal(t, ".", packageOf("main.go"))
	assert.Equal(t, "./internal/max", packageOf("internal/max/max.go"))
	assert.Equal(t, "./-x", packageOf("-x/y.go"))
	assert.Equal(t, "/abs", packageOf("/abs/x.go"))
}

func TestTestChecks_RunOncePerPackage(t *testing.T) {
	checks := newTestChecks()
	failure := errors.New("tests fail")
	calls := 0

	run := func() error {
		calls++
		return failure
	}

	assert.Equal(t, failure, checks.check("/p/a", run))
	assert.Equal(t, failure, checks.check("/p/a", run))
	assert.NoError(t, checks.check("/p/b", func() error { return nil }))
	assert.Equal(t, 1, calls)
}

func TestUntestedPackages_NoneIsNil(t *testing.T) {
	var untested untestedPackages

	assert.NoError(t, untested.err())
}
