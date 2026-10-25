package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
)

func TestRunCmd_GoCacheFlags(t *testing.T) {
	for name, tt := range map[string]struct {
		args []string
		env  map[string]string
		want domain.GoCache
	}{
		"defaults": {
			args: []string{"run", "./..."},
			want: domain.GoCache{Mode: domain.GoCacheIsolated, SizeMB: 2048},
		},
		"flags": {
			args: []string{"run", "--go-cache", "shared", "--go-cache-size", "10", "--go-cache-prewarm", "./..."},
			want: domain.GoCache{Mode: domain.GoCacheShared, SizeMB: 10, Prewarm: true},
		},
		"env": {
			args: []string{"run", "./..."},
			env:  map[string]string{"GOOZE_RUN_GO_CACHE": "shared", "GOOZE_RUN_GO_CACHE_SIZE": "0", "GOOZE_RUN_GO_CACHE_PREWARM": "true"},
			want: domain.GoCache{Mode: domain.GoCacheShared, SizeMB: 0, Prewarm: true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			mockWorkflow := domainmocks.NewMockWorkflow(t)

			cmd := newRootCmd()
			cmd.AddCommand(newRunCmd())
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			originalWorkflow := workflow
			workflow = mockWorkflow
			defer func() { workflow = originalWorkflow }()
			// Rebind the keys to fresh, unset flags so later tests see the defaults.
			defer newRunCmd()

			mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
				return args.GoCache == tt.want
			})).Return(nil)

			cmd.SetArgs(tt.args)
			require.NoError(t, cmd.Execute())
			mockWorkflow.AssertExpectations(t)
		})
	}
}

func TestRunCmd_RejectsBadGoCache(t *testing.T) {
	for args, want := range map[string]string{
		"--go-cache=private": `invalid run.go_cache: "private" is not one of isolated, shared`,
		"--go-cache-size=-1": "invalid run.go_cache_size: -1 is out of range: must be at least 0",
	} {
		cmd := newRootCmd()
		cmd.AddCommand(newRunCmd())
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		cmd.SetArgs([]string{"run", args, "./..."})
		require.EqualError(t, cmd.Execute(), want)
		newRunCmd()
	}
}
