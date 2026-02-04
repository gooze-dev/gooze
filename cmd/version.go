package cmd

import (
	"runtime/debug"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the version information",
		Long:  "Displays the build version and Go version used to build this tool.",
		Run: func(cmd *cobra.Command, _ []string) {
			info, ok := debug.ReadBuildInfo()
			if !ok || info.Main.Version == "" {
				cmd.Println("version: unknown")
				return
			}

			cmd.Println("tool version\t", info.Main.Version)
			cmd.Println("go version\t", info.GoVersion)
		},
	}
}

// versionCmd represents the version command.
var versionCmd = newVersionCmd()

func init() {
	rootCmd.AddCommand(versionCmd)
}
