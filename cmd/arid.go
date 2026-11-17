package cmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gooze.dev/pkg/gooze/internal/domain"
	"gooze.dev/pkg/gooze/pkg/config"
)

const aridFlagName = "arid"

var runAridFlag bool

func addAridFlag(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&runAridFlag, aridFlagName, schemaDefault[bool](config.KeyAridEnabled),
		"don't mutate arid code: logging, flag and metrics calls, time values, and the conditions that only guard them (see arid.rules and arid.calls)")
	bindFlagToConfig(cmd.Flags().Lookup(aridFlagName), config.KeyAridEnabled)
}

// aridOptions reads the arid settings; a rule gooze doesn't have is a usage
// error.
func aridOptions() (domain.AridOptions, error) {
	rules := viper.GetStringSlice(config.KeyAridRules)

	field, _ := config.Lookup(config.KeyAridRules)
	for _, rule := range rules {
		if !slices.Contains(field.Enum, rule) {
			return domain.AridOptions{}, usageError(fmt.Errorf("invalid %s: %q is not one of %s", config.KeyAridRules, rule, strings.Join(field.Enum, ", ")))
		}
	}

	return domain.AridOptions{
		Enabled: viper.GetBool(config.KeyAridEnabled),
		Rules:   rules,
		Calls:   viper.GetStringSlice(config.KeyAridCalls),
	}, nil
}
