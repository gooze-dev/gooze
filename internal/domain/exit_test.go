package domain_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestWithExitCode(t *testing.T) {
	assert.NoError(t, domain.WithExitCode(m.ExitUsage, nil))

	cause := errors.New("bad flag")
	err := domain.WithExitCode(m.ExitUsage, cause)

	var coded *m.CodedError
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, m.ExitUsage, coded.Code)
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, "bad flag", err.Error())
}

// exitCodeOf returns the exit code err carries, or ExitFailure when none.
func exitCodeOf(err error) m.ExitCode {
	var coded *m.CodedError
	if errors.As(err, &coded) {
		return coded.Code
	}

	return m.ExitFailure
}
