package model

// ExitCode is the process exit status of a gooze command, one per kind of
// outcome, so scripts and CI can tell them apart.
type ExitCode int

// Exit codes of gooze commands.
const (
	// ExitOK: the command completed, and the score met every threshold set.
	ExitOK ExitCode = 0
	// ExitFailure: an unexpected failure, such as an I/O or report error.
	ExitFailure ExitCode = 1
	// ExitUsage: a bad flag, argument or config value.
	ExitUsage ExitCode = 2
	// ExitBelowThreshold: the run completed, but its score is below a
	// threshold.
	ExitBelowThreshold ExitCode = 3
	// ExitGit: --diff could not compare against git.
	ExitGit ExitCode = 4
	// ExitRegistry: pushing or pulling reports to an OCI registry failed.
	ExitRegistry ExitCode = 5
	// ExitNoReports: there are no reports to view or merge.
	ExitNoReports ExitCode = 6
	// ExitInterrupted: the command was stopped by SIGINT or SIGTERM, as a
	// shell reports for SIGINT (128+2).
	ExitInterrupted ExitCode = 130
)

// CodedError is an error that sets the exit code of the command returning it.
type CodedError struct {
	Code ExitCode
	Err  error
}

func (e *CodedError) Error() string { return e.Err.Error() }

// Unwrap returns the wrapped error.
func (e *CodedError) Unwrap() error { return e.Err }
