package cmd

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestExitCode(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	failure := errors.New("disk full")

	for name, tc := range map[string]struct {
		ctx       context.Context
		err       error
		beforeRun bool
		want      m.ExitCode
	}{
		"success":                 {ctx: context.Background(), want: m.ExitOK},
		"success after interrupt": {ctx: cancelled, want: m.ExitOK},
		"failure":                 {ctx: context.Background(), err: failure, want: m.ExitFailure},
		"rejected by cobra":       {ctx: context.Background(), err: failure, beforeRun: true, want: m.ExitUsage},
		"coded":                   {ctx: context.Background(), err: fmt.Errorf("run: %w", domain.WithExitCode(m.ExitRegistry, failure)), want: m.ExitRegistry},
		"coded before run":        {ctx: context.Background(), err: domain.WithExitCode(m.ExitNoReports, failure), beforeRun: true, want: m.ExitNoReports},
		"interrupted":             {ctx: cancelled, err: fmt.Errorf("run: %w", context.Canceled), want: m.ExitInterrupted},
		"interrupted coded":       {ctx: cancelled, err: domain.WithExitCode(m.ExitBelowThreshold, failure), want: m.ExitInterrupted},
		"no git":                  {ctx: context.Background(), err: fmt.Errorf("diff: %w", domain.ErrGitNotInstalled), want: m.ExitGit},
		"not a repository":        {ctx: context.Background(), err: fmt.Errorf("diff: %w", domain.ErrNotGitRepository), want: m.ExitGit},
		"unknown ref":             {ctx: context.Background(), err: fmt.Errorf("diff: %w", domain.ErrUnknownRef), want: m.ExitGit},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, exitCode(tc.ctx, tc.err, tc.beforeRun))
		})
	}
}
