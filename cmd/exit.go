package cmd

import (
	"context"
	"errors"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

// usageError marks err as a bad flag, argument or config value.
func usageError(err error) error { return domain.WithExitCode(m.ExitUsage, err) }

// exitCode returns the process exit status for the error a command returned.
// ctx is the command's signal context: once it is done, the command was
// interrupted, whatever error that caused. beforeRun is true when the error
// came before any command ran (cobra rejected a flag, an argument or the
// command name), which is a usage error.
func exitCode(ctx context.Context, err error, beforeRun bool) m.ExitCode {
	var coded *m.CodedError

	switch {
	case err == nil:
		return m.ExitOK
	case ctx.Err() != nil:
		return m.ExitInterrupted
	case errors.As(err, &coded):
		return coded.Code
	case errors.Is(err, domain.ErrGitNotInstalled), errors.Is(err, domain.ErrNotGitRepository), errors.Is(err, domain.ErrUnknownRef):
		return m.ExitGit
	case beforeRun:
		return m.ExitUsage
	default:
		return m.ExitFailure
	}
}
