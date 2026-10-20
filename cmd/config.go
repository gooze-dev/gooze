package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"

	"gooze.dev/pkg/gooze/pkg/config"
)

const (
	configFolderPath = "."

	outputFlagName      = "output"
	noCacheFlagName     = "no-cache"
	excludeFlagName     = "exclude"
	runParallelFlagName = "parallel"

	mutationTimeoutFlagName = "mutation-timeout"

	coverageProfileFlagName = "coverage-profile"

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
		if field.Kind != config.KindInt || (field.Min == nil && field.Max == nil) {
			continue
		}

		if err := field.CheckInt(viper.GetInt(key)); err != nil {
			return fmt.Errorf("invalid %s: %w", key, err)
		}
	}

	return nil
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
