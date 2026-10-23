package report

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

const formatFlagName = "format"

// writesOutput holds the formats that write more than the text summary.
var writesOutput = map[domain.ViewFormat]bool{
	domain.FormatGitHub: true, domain.FormatGitLab: true, domain.FormatSARIF: true, domain.FormatCodeClimate: true,
}

// addFormatFlag adds --format, whose default comes from report.format in the
// schema; gooze.yaml and GOOZE_REPORT_FORMAT set it without the flag.
func addFormatFlag(cmd *cobra.Command) {
	field, _ := config.Lookup(config.KeyReportFormat)
	value, _ := field.Default.(string)

	cmd.Flags().String(formatFlagName, value, "how to write surviving mutants: "+strings.Join(field.Enum, ", ")+" (config report.format)")
}

// readFormat returns the --format flag or else report.format, checked against
// the schema's values.
func readFormat(cmd *cobra.Command) (domain.ViewFormat, error) {
	field, _ := config.Lookup(config.KeyReportFormat)

	if cmd.Flags().Changed(formatFlagName) {
		value, _ := cmd.Flags().GetString(formatFlagName)
		if !slices.Contains(field.Enum, value) {
			return "", domain.WithExitCode(m.ExitUsage,
				fmt.Errorf("invalid --%s %q: must be one of %s", formatFlagName, value, strings.Join(field.Enum, ", ")))
		}

		return domain.ViewFormat(value), nil
	}

	return domain.ViewFormat(viper.GetString(config.KeyReportFormat)), nil
}

// showPrefix returns the working directory's path from the git repository
// root, with a trailing slash ("" at the root). A variable so tests can stub
// git.
var showPrefix = func(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-prefix").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-prefix: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// repoPathPrefix returns what to prepend to report paths, which are relative
// to the working directory, so GitHub, GitLab and SARIF tools find the files:
// the directory's path in its git repository, else in $GITHUB_WORKSPACE or
// $CI_PROJECT_DIR (GitLab's checkout), else nothing.
func repoPathPrefix(ctx context.Context) string {
	if prefix, err := showPrefix(ctx); err == nil {
		return prefix
	}

	return prefixIn(ciWorkspace())
}

// ciWorkspace returns the CI job's checkout directory, or "" outside CI.
func ciWorkspace() string {
	if workspace := os.Getenv("GITHUB_WORKSPACE"); workspace != "" {
		return workspace
	}

	return os.Getenv("CI_PROJECT_DIR")
}

// prefixIn returns the working directory's path in workspace with a trailing
// slash, or "" at its root, outside it, or without a workspace.
func prefixIn(workspace string) string {
	// Equivalent: no name sorts before "", and filepath.Rel rejects an empty
	// base below anyway.
	if workspace == "" { //gooze:ignore comparison
		return ""
	}

	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	rel, err := filepath.Rel(workspace, wd)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}

	return filepath.ToSlash(rel) + "/"
}

// readBuildInfo is debug.ReadBuildInfo, replaced in tests.
var readBuildInfo = debug.ReadBuildInfo

// toolVersion returns gooze's module version, for SARIF output.
func toolVersion() string {
	if info, ok := readBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}

	return "(devel)"
}
