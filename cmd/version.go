package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the version information",
		Long:  "Displays the build version and Go version used to build this tool.",
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), currentVersion())
		},
	}
}

// currentVersion returns this binary's version line.
func currentVersion() string {
	return versionFrom(debug.ReadBuildInfo())
}

// versionFrom formats the result of debug.ReadBuildInfo.
func versionFrom(info *debug.BuildInfo, ok bool) string {
	if !ok {
		return "gooze (unknown version)"
	}

	return versionLine(info)
}

// versionLine formats build info as "gooze v1.2.3 (go1.27.3)".
func versionLine(info *debug.BuildInfo) string {
	version := info.Main.Version
	if version == "" {
		version = "(unknown version)"
	}

	return fmt.Sprintf("gooze %s (%s)", version, info.GoVersion)
}

// versionCmd represents the version command.
var versionCmd = newVersionCmd()

func init() {
	rootCmd.AddCommand(versionCmd)
}
