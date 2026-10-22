package cmd

import (
	"bytes"
	"slices"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
)

// setConfigEnumBounds sets mutagen.comparison.enum_bounds in viper's
// config-file layer, as gooze.yaml does.
func setConfigEnumBounds(t *testing.T, on bool) {
	t.Helper()

	set := func(v bool) map[string]any {
		return map[string]any{"mutagen": map[string]any{"comparison": map[string]any{"enum_bounds": v}}}
	}

	require.NoError(t, viper.MergeConfigMap(set(on)))
	t.Cleanup(func() { _ = viper.MergeConfigMap(set(false)) })
}

func TestRunCmd_EnumBoundsConfig(t *testing.T) {
	for _, tt := range []struct {
		name   string
		args   []string
		env    string
		config bool // enum_bounds: true in the config-file layer
		want   bool
	}{
		{name: "default", args: []string{"run", "./..."}, want: false},
		{name: "config file", args: []string{"run", "./..."}, config: true, want: true},
		{name: "env", args: []string{"run", "./..."}, env: "true", want: true},
		{name: "env over config file", args: []string{"run", "./..."}, env: "false", config: true, want: false},
		{name: "estimate", args: []string{"run", "--estimate", "./..."}, env: "true", want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config {
				setConfigEnumBounds(t, true)
			}

			if tt.env != "" {
				t.Setenv("GOOZE_MUTAGEN_COMPARISON_ENUM_BOUNDS", tt.env)
			}

			mockWorkflow := domainmocks.NewMockWorkflow(t)

			cmd := newRootCmd()
			cmd.AddCommand(newRunCmd())
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			originalWorkflow := workflow
			workflow = mockWorkflow
			defer func() { workflow = originalWorkflow }()

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

func TestRunCmd_NoEnumBoundsFlag(t *testing.T) {
	require.Nil(t, newRunCmd().Flags().Lookup("enum-bounds"))
}
