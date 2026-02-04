package cmd

import (
	"errors"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	configVersionKey     = "version"
	currentConfigVersion = 1

	configBaseName   = "gooze"
	configFileName   = configBaseName + ".yaml"
	configFolderPath = "."

	outputFlagName      = "output"
	noCacheFlagName     = "no-cache"
	excludeFlagName     = "exclude"
	runParallelFlagName = "parallel"

	mutationTimeoutFlagName = "mutation-timeout"

	runParallelConfigKey = "run.parallel"
	mutationTimeoutKey   = "run.mutation_timeout"
	excludeConfigKey     = "paths.exclude"

	defaultMutationTimeout = time.Minute * 2

	defaultReportsDir  = ".gooze-reports"
	defaultNoCache     = false
	defaultRunParallel = 1

	envPrefix = "GOOZE"

	logFilenameKey   = "log.filename"
	logLevelKey      = "log.level"
	logVerboseKey    = "log.verbose"
	logMaxSizeKey    = "log.max_size"
	logMaxBackupsKey = "log.max_backups"
	logMaxAgeKey     = "log.max_age"
	logCompressKey   = "log.compress"

	defaultLogFilename   = ".gooze.log"
	defaultLogLevel      = int(slog.LevelInfo)
	defaultLogVerbose    = false
	defaultLogMaxSize    = 10
	defaultLogMaxBackups = 3
	defaultLogMaxAge     = 28
	defaultLogCompress   = true
)

var globalLogger *slog.Logger

func init() {
	viper.SetConfigName(configBaseName)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configFolderPath)
	viper.SetConfigFile(filepath.Join(configFolderPath, configFileName))
	viper.AutomaticEnv()
	viper.SetEnvPrefix(envPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	viper.SetDefault(configVersionKey, currentConfigVersion)
	viper.SetDefault(outputFlagName, defaultReportsDir)
	viper.SetDefault(noCacheFlagName, defaultNoCache)
	viper.SetDefault(runParallelConfigKey, defaultRunParallel)
	viper.SetDefault(mutationTimeoutKey, int64(defaultMutationTimeout.Seconds()))
	viper.SetDefault(excludeConfigKey, []string{})

	// Logging defaults (used by config/env and as fallbacks for flags).
	viper.SetDefault(logFilenameKey, defaultLogFilename)
	viper.SetDefault(logLevelKey, defaultLogLevel)
	viper.SetDefault(logVerboseKey, defaultLogVerbose)
	viper.SetDefault(logMaxSizeKey, defaultLogMaxSize)
	viper.SetDefault(logMaxBackupsKey, defaultLogMaxBackups)
	viper.SetDefault(logMaxAgeKey, defaultLogMaxAge)
	viper.SetDefault(logCompressKey, defaultLogCompress)

	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			return
		}

		return
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
		logPath = viper.GetString(logFilenameKey)
	}

	if strings.TrimSpace(logPath) == "" {
		logPath = defaultLogFilename
	}

	var logLevel slog.Level
	if verbose {
		logLevel = slog.LevelDebug
	} else {
		logLevel = parseSlogLevel(viper.GetString(logLevelKey), slog.LevelInfo)
	}

	logWriter := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    viper.GetInt(logMaxSizeKey),
		MaxBackups: viper.GetInt(logMaxBackupsKey),
		MaxAge:     viper.GetInt(logMaxAgeKey),
		Compress:   viper.GetBool(logCompressKey),
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
