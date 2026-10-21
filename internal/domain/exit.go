package domain

import m "gooze.dev/pkg/gooze/internal/model"

// WithExitCode makes err end the command with code; nil stays nil.
func WithExitCode(code m.ExitCode, err error) error {
	if err == nil {
		return nil
	}

	return &m.CodedError{Code: code, Err: err}
}
