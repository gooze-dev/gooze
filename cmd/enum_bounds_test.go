package cmd

import (
	"bytes"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
)

func TestRunCmd_EnumBoundsFlag(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		env  string
		want bool
	}{
		{name: "default", args: []string{"run", "./..."}, want: false},
		{name: "flag", args: []string{"run", "--enum-bounds", "./..."}, want: true},
		{name: "env", args: []string{"run", "./..."}, env: "true", want: true},
		{name: "estimate", args: []string{"run", "--estimate", "--enum-bounds", "./..."}, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("GOOZE_RUN_ENUM_BOUNDS", tt.env)
			}

			mockWorkflow := domainmocks.NewMockWorkflow(t)

			cmd := newRootCmd()
			cmd.AddCommand(newRunCmd())
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			originalWorkflow := workflow
			workflow = mockWorkflow
			defer func() { workflow = originalWorkflow }()
			// Rebind run.enum_bounds to a fresh, unset flag so later tests see the default.
			defer newRunCmd()

			if slices.Contains(tt.args, "--estimate") {
				mockWorkflow.On("Estimate", mock.Anything, mock.MatchedBy(func(args domain.EstimateArgs) bool {
					return args.EnumBounds == tt.want
				})).Return(nil)
			} else {
				mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
					return args.EnumBounds == tt.want
				})).Return(nil)
			}

			cmd.SetArgs(tt.args)
			require.NoError(t, cmd.Execute())
			mockWorkflow.AssertExpectations(t)
		})
	}
}

func TestRunCmd_EnumBoundsFlagDefaultMatchesSchema(t *testing.T) {
	assert.Equal(t, "false", newRunCmd().Flags().Lookup(enumBoundsFlagName).DefValue)
}
