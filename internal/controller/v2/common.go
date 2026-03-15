package v2

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	m "gooze.dev/pkg/gooze/internal/model"
)

func parsePaths(args []string) []m.Path {
	paths := make([]m.Path, 0, len(args))
	for _, arg := range args {
		paths = append(paths, m.Path(arg))
	}

	return paths
}

func bindStringFlag(
	cmd *cobra.Command,
	flagName string,
	defaultValue string,
	description string,
	variable *string,
) {
	viper.SetDefault(flagName, defaultValue)
	cmd.PersistentFlags().StringVar(variable, flagName, defaultValue, description)
	viper.BindPFlag(flagName, cmd.Flags().Lookup(flagName))
}

func bindBoolFlag(
	cmd *cobra.Command,
	flagName string,
	defaultValue bool,
	description string,
	variable *bool,
) {
	viper.SetDefault(flagName, defaultValue)
	cmd.PersistentFlags().BoolVar(variable, flagName, defaultValue, description)
	viper.BindPFlag(flagName, cmd.Flags().Lookup(flagName))
}

func bindStringArrayFlag(
	cmd *cobra.Command,
	flagName string,
	defaultValue []string,
	description string,
	variable *[]string,
) {
	viper.SetDefault(flagName, defaultValue)
	cmd.PersistentFlags().StringArrayVar(variable, flagName, defaultValue, description)
	viper.BindPFlag(flagName, cmd.Flags().Lookup(flagName))
}
