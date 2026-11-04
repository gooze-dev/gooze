package domain

import (
	"fmt"
	"strings"
)

// goozeTestFlags are the go test flags gooze sets itself (-count, -vet,
// -overlay) or can't run with: -json changes the output gooze reads, and -c
// and -o build a test binary instead of running the tests.
var goozeTestFlags = map[string]bool{"json": true, "count": true, "vet": true, "overlay": true, "c": true, "o": true}

// ParseTestFlags returns the go test flags of run.test_flags: entries split at
// spaces. Each one must be a flag, with any value after =, so no package or
// stray value reaches go test, and none may be a flag gooze sets itself.
func ParseTestFlags(entries []string) ([]string, error) {
	var flags []string

	for _, entry := range entries {
		for _, flag := range strings.Fields(entry) {
			if !strings.HasPrefix(flag, "-") {
				return nil, notAFlag(flag)
			}

			name, _, _ := strings.Cut(strings.TrimLeft(flag, "-"), "=")
			if goozeTestFlags[name] {
				return nil, fmt.Errorf("run.test_flags: gooze sets -%s itself or can't run with it", name)
			}

			flags = append(flags, flag)
		}
	}

	return flags, nil
}

// notAFlag explains a value that isn't a flag: after a flag it is the flag's
// value, which needs =.
func notAFlag(value string) error {
	if strings.ContainsAny(value, "./") {
		return fmt.Errorf("run.test_flags: %q is not a flag", value)
	}

	return fmt.Errorf("run.test_flags: %q is not a flag; write a flag's value with =, as in -skip=%s", value, value)
}

// TestCommand takes the extra flags of the go test commands that test mutants.
type TestCommand interface {
	SetTestFlags(flags []string)
}

// WithTestFlags passes each run's TestArgs.TestFlags to command.
func WithTestFlags(command TestCommand) WorkflowOption {
	return func(w *workflow) {
		w.testCommand = command
	}
}
