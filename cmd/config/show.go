package config

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gooze.dev/pkg/gooze/pkg/config"
)

func newShowCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print every config key with its value and where it comes from",
		Long: `Print every config key with its effective value, where that value comes from
(flag, env, gooze.yaml or default, in that order of precedence) and its
environment variable. Only flags available to every command can show here;
flags of gooze run apply to gooze run.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			flagged := flaggedKeys(cmd.Flags(), deps.FlagKeys)

			// Equivalent: these numbers only set the table's spacing, not its content.
			table := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0) //gooze:ignore numbers
			_, _ = fmt.Fprintln(table, "KEY\tVALUE\tSOURCE\tENV")

			for _, key := range config.Keys() {
				env := envName(deps.EnvPrefix, key)
				_, _ = fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", key, showValue(viper.Get(key)), source(flagged[key], os.Getenv(env) != "", viper.InConfig(key)), env)
			}

			return table.Flush()
		},
	}
}

// flaggedKeys returns the config keys of the flags set on the command line.
func flaggedKeys(flags *pflag.FlagSet, keys map[string]string) map[string]bool {
	flagged := map[string]bool{}

	flags.Visit(func(flag *pflag.Flag) {
		flagged[keys[flag.Name]] = true
	})

	return flagged
}

// source names where a key's value comes from, in viper's order of precedence.
func source(flag, env, file bool) string {
	switch {
	case flag:
		return "flag"
	case env:
		return "env"
	case file:
		return config.FileName
	default:
		return "default"
	}
}

func envName(prefix, key string) string {
	return prefix + "_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(key))
}

// showValue returns the text of value, quoting an empty one so the column shows it.
func showValue(value any) string {
	text := fmt.Sprint(value)
	if text == "" {
		return `""`
	}

	return text
}
