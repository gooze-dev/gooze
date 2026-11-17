package cmd

import (
	"errors"

	"github.com/spf13/cobra"
	"gooze.dev/pkg/gooze/internal/domain"
)

var runIgnoredFlag bool

// errIgnoredWithEstimate rejects --ignored with --estimate: each replaces
// the run with a listing of its own.
var errIgnoredWithEstimate = errors.New("--ignored lists the ignored mutants; --estimate counts the tested ones: use one of them")

func addIgnoredFlag(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&runIgnoredFlag, "ignored", false,
		"list the mutants that //gooze:ignore directives keep from being tested, with their reasons, without running tests")
}

// runIgnored lists the mutants the directives ignore in the sources and
// mutants estimateArgs selects, on the command's standard output.
func runIgnored(cmd *cobra.Command, estimateArgs domain.EstimateArgs) error {
	if runEstimateFlag {
		return usageError(errIgnoredWithEstimate)
	}

	return workflow.Ignored(cmd.Context(), domain.IgnoredArgs{EstimateArgs: estimateArgs, Output: cmd.OutOrStdout()})
}
