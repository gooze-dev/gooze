package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cast"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"

	"gooze.dev/pkg/gooze/pkg/config"
)

const (
	configFolderPath = "."

	outputFlagName      = "output"
	noCacheFlagName     = "no-cache"
	noUpdateCheckFlag   = "no-update-check"
	excludeFlagName     = "exclude"
	runParallelFlagName = "parallel"

	mutationTimeoutFlagName = "mutation-timeout"

	coverageProfileFlagName = "coverage-profile"

	diffFlagName     = "diff"
	runFlagName      = "run"
	mutagensFlagName = "mutagens"

	workspaceFlagName = "workspace"

	envPrefix = "GOOZE"
)

var globalLogger *slog.Logger

func init() {
	viper.SetConfigType("yaml")
	viper.SetConfigFile(filepath.Join(configFolderPath, config.FileName))
	viper.AutomaticEnv()
	viper.SetEnvPrefix(envPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	for _, key := range config.Keys() {
		field, _ := config.Lookup(key)
		viper.SetDefault(key, field.Default)
	}

	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		// A missing config file is expected; only warn on real read errors.
		if errors.As(err, &notFound) || errors.Is(err, os.ErrNotExist) {
			return
		}

		slog.Warn("failed to read config file", "error", err)
	}
}

// validateConfig checks the effective value of every bounded key (from flags,
// environment, gooze.yaml or defaults) against the schema.
func validateConfig() error {
	for _, key := range config.Keys() {
		field, _ := config.Lookup(key)
		if err := validateValue(field, key); err != nil {
			return usageError(fmt.Errorf("invalid %s: %w", key, err))
		}
	}

	return nil
}

// validateValue checks the value of key against field's bounds: a whole
// number for a bounded KindInt field, a number for a KindFloat one, one of the
// listed values for a KindEnum one that doesn't take integers.
func validateValue(field config.Field, key string) error {
	switch field.Kind {
	case config.KindEnum:
		if value := viper.GetString(key); !field.IntEnum && !slices.Contains(field.Enum, value) {
			return fmt.Errorf("%q is not one of %s", value, strings.Join(field.Enum, ", "))
		}

		return nil
	case config.KindFloat:
		x, err := cast.ToFloat64E(viper.Get(key))
		if err != nil {
			return fmt.Errorf("%q is not a number", viper.GetString(key))
		}

		return field.CheckFloat(x)
	case config.KindInt:
		return field.CheckInt(viper.GetInt(key))
	default:
		return nil
	}
}

func parseSlogLevel(value string, defaultLevel slog.Level) slog.Level {
	level := strings.ToLower(strings.TrimSpace(value))
	if level == "" {
		return defaultLevel
	}

	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}

	// Allow numeric slog levels as well (e.g. -4 for debug).
	if n, err := strconv.Atoi(level); err == nil {
		return slog.Level(n)
	}

	return defaultLevel
}

// configureLogger configures the global slog logger.
//
// By default it logs at Info; if verbose is true it logs at Debug.
func configureLogger(logPath string, verbose bool) {
	if strings.TrimSpace(logPath) == "" {
		logPath = viper.GetString(config.KeyLogFilename)
	}

	if strings.TrimSpace(logPath) == "" {
		logPath = defaultLogFilename()
	}

	var logLevel slog.Level
	if verbose {
		logLevel = slog.LevelDebug
	} else {
		logLevel = parseSlogLevel(viper.GetString(config.KeyLogLevel), slog.LevelInfo)
	}

	logWriter := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    viper.GetInt(config.KeyLogMaxSize),
		MaxBackups: viper.GetInt(config.KeyLogMaxBackups),
		MaxAge:     viper.GetInt(config.KeyLogMaxAge),
		Compress:   viper.GetBool(config.KeyLogCompress),
	}

	// Create a JSON handler that writes to the file
	handler := slog.NewTextHandler(logWriter, &slog.HandlerOptions{
		AddSource: true,
		Level:     logLevel,
	})

	// Create a new logger with the file handler and set it as the global logger
	globalLogger = slog.New(handler)
	slog.SetDefault(globalLogger)
}

func defaultLogFilename() string {
	field, _ := config.Lookup(config.KeyLogFilename)
	name, _ := field.Default.(string)

	return name
}
