package report

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cast"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

// ThresholdFlags are a command's quality gate flags, shared by gooze run
// and report view and merge: --<prefix>-total, --<prefix>-average,
// --<prefix>-file and, under threshold, --<prefix>-max-survivors. Each
// defaults to the config key of the same name under key.
type ThresholdFlags struct {
	key       string
	prefix    string
	survivors bool
}

const maxSurvivors = "max_survivors"

// thresholdHelp describes each threshold flag.
var thresholdHelp = map[string]string{
	"total":   "the mutation score",
	"average": "the average of the file scores",
	"file":    "any file's score",
}

// thresholdNames are the thresholds in the order of domain.Thresholds.
var thresholdNames = []string{"total", "average", "file"}

// AddThresholdFlags adds the threshold flags named after prefix (such as
// "threshold" or "threshold-diff") for the config keys under key (such as
// "threshold" or "threshold.diff").
func AddThresholdFlags(cmd *cobra.Command, key, prefix string) ThresholdFlags {
	for _, name := range thresholdNames {
		cmd.Flags().Float64(prefix+"-"+name, 0, fmt.Sprintf("exit with code 3 when %s is below this percent (config %s.%s; 0 is off)", thresholdHelp[name], key, name))
	}

	_, survivors := config.Lookup(key + "." + maxSurvivors)
	if survivors {
		cmd.Flags().Int(prefix+"-max-survivors", 0, fmt.Sprintf("exit with code 3 when more mutants than this survive (config %s.%s; 0 is off)", key, maxSurvivors))
	}

	return ThresholdFlags{key: key, prefix: prefix, survivors: survivors}
}

// Read returns the thresholds cmd's flags set, or else the config, checked to
// be percents.
func (f ThresholdFlags) Read(cmd *cobra.Command) (domain.Thresholds, error) {
	t := domain.Thresholds{Key: f.key}
	values := []*float64{&t.Total, &t.Average, &t.File}

	for i, name := range thresholdNames {
		value, err := f.value(cmd, name)
		if err != nil {
			return domain.Thresholds{}, domain.WithExitCode(m.ExitUsage, err)
		}

		*values[i] = value
	}

	if f.survivors {
		value, err := f.survivorCap(cmd)
		if err != nil {
			return domain.Thresholds{}, domain.WithExitCode(m.ExitUsage, err)
		}

		t.MaxSurvivors = value
	}

	return t, nil
}

// value returns the threshold name from its flag or the config.
func (f ThresholdFlags) value(cmd *cobra.Command, name string) (float64, error) {
	key := f.key + "." + name
	field, _ := config.Lookup(key)

	if flag := f.prefix + "-" + name; cmd.Flags().Changed(flag) {
		value, _ := cmd.Flags().GetFloat64(flag)
		if err := field.CheckFloat(value); err != nil {
			return 0, fmt.Errorf("invalid --%s: %w", flag, err)
		}

		return value, nil
	}

	value, err := cast.ToFloat64E(viper.Get(key))
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %q is not a number", key, viper.GetString(key))
	}

	if err := field.CheckFloat(value); err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}

	return value, nil
}

// survivorCap returns threshold.max_survivors from its flag or the config.
func (f ThresholdFlags) survivorCap(cmd *cobra.Command) (int, error) {
	key := f.key + "." + maxSurvivors
	field, _ := config.Lookup(key)

	if flag := f.prefix + "-max-survivors"; cmd.Flags().Changed(flag) {
		value, _ := cmd.Flags().GetInt(flag)
		if err := field.CheckInt(value); err != nil {
			return 0, fmt.Errorf("invalid --%s: %w", flag, err)
		}

		return value, nil
	}

	if !viper.IsSet(key) {
		return 0, nil
	}

	// Base 10: cast would read "010" as octal.
	text := strings.TrimSpace(viper.GetString(key))

	value, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %q is not a whole number", key, text)
	}

	if err := field.CheckInt(value); err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}

	return value, nil
}

// ReadGates returns the plain and the --diff thresholds; both share
// threshold.max_survivors.
func ReadGates(cmd *cobra.Command, plain, diff ThresholdFlags) (domain.Thresholds, domain.Thresholds, error) {
	minimums, err := plain.Read(cmd)
	if err != nil {
		return domain.Thresholds{}, domain.Thresholds{}, err
	}

	diffMinimums, err := diff.Read(cmd)
	if err != nil {
		return domain.Thresholds{}, domain.Thresholds{}, err
	}

	diffMinimums.MaxSurvivors = minimums.MaxSurvivors

	return minimums, diffMinimums, nil
}
