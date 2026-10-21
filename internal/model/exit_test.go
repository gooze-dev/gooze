package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	m "gooze.dev/pkg/gooze/internal/model"
)

// The codes are documented for scripts and CI, so they must never change.
func TestExitCodesAreStable(t *testing.T) {
	codes := []m.ExitCode{
		m.ExitOK, m.ExitFailure, m.ExitUsage, m.ExitBelowThreshold,
		m.ExitGit, m.ExitRegistry, m.ExitNoReports, m.ExitInterrupted,
	}

	got := make([]int, 0, len(codes))
	for _, code := range codes {
		got = append(got, int(code))
	}

	assert.Equal(t, []int{0, 1, 2, 3, 4, 5, 6, 130}, got)
}

func TestCodedError(t *testing.T) {
	cause := assert.AnError
	err := &m.CodedError{Code: m.ExitUsage, Err: cause}

	assert.Equal(t, cause.Error(), err.Error())
	require.ErrorIs(t, err, cause)
}
