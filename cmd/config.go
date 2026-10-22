package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
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

// validateConfig checks the effective value of every key (from flags,
// environment, gooze.yaml or defaults) against the schema: its type, and its
// bounds or listed values.
func validateConfig() error {
	for _, key := range config.Keys() {
		field, _ := config.Lookup(key)
		if err := validateValue(field, key); err != nil {
			return usageError(fmt.Errorf("invalid %s: %w", key, err))
		}
	}

	return nil
}

// validateValue checks the value of key against field: a whole number within
// the bounds for KindInt, a number within them for KindFloat, true or false
// for KindBool, a listed value (or, with IntEnum, an integer) for KindEnum, a
// single value for KindString and a list of single values for KindStringList.
func validateValue(field config.Field, key string) error {
	value := viper.Get(key)

	switch field.Kind {
	case config.KindInt:
		n, err := wholeNumber(value)
		if err != nil {
			return err
		}

		return field.CheckInt(n)
	case config.KindFloat:
		x, err := cast.ToFloat64E(value)
		if err != nil {
			return fmt.Errorf("%s is not a number", describe(value))
		}

		return field.CheckFloat(x)
	case config.KindBool:
		return checkBool(value)
	case config.KindEnum:
		return checkEnum(field, value)
	case config.KindString:
		return checkSingle(value)
	case config.KindStringList:
		return checkStringList(value)
	case config.KindObject:
		// config.Keys lists only values, never objects.
		return nil
	}

	return nil
}

func checkBool(value any) error {
	if _, err := cast.ToBoolE(value); err != nil {
		return fmt.Errorf("%s is not true or false", describe(value))
	}

	return nil
}

func checkSingle(value any) error {
	if !isScalar(value) {
		return fmt.Errorf("%s is not a single value", describe(value))
	}

	return nil
}

// wholeNumber returns value as an int, rejecting booleans and numbers with a
// fractional part, which cast would silently turn into 1 or truncate. On
// error the int is meaningless.
func wholeNumber(value any) (int, error) {
	// Strings (env vars, quoted YAML) are read in base 10: cast reads a leading
	// 0 as octal and 0x as hex, so "010" would be 8.
	if s, ok := value.(string); ok {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return n, fmt.Errorf("%s is not a whole number", describe(value))
		}

		return n, nil
	}

	n, err := cast.ToIntE(value)
	_, isBool := value.(bool)
	x, isFloat := value.(float64)

	if err != nil || isBool || isFloat && x != math.Trunc(x) {
		return n, fmt.Errorf("%s is not a whole number", describe(value))
	}

	return n, nil
}

// configInt returns the whole-number value of key, read the way
// validateConfig checks it (strings in base 10).
func configInt(key string) int {
	n, _ := wholeNumber(viper.Get(key))

	return n
}

// checkEnum accepts one of field.Enum as written, or, for an IntEnum field
// such as log.level, a listed name in any case, an integer, or nothing (the
// default), the way parseSlogLevel reads it.
func checkEnum(field config.Field, value any) error {
	names := strings.Join(field.Enum, ", ")

	if !field.IntEnum {
		if s, ok := value.(string); ok && slices.Contains(field.Enum, s) {
			return nil
		}

		return fmt.Errorf("%s is not one of %s", describe(value), names)
	}

	if isScalar(value) {
		s := strings.ToLower(strings.TrimSpace(cast.ToString(value)))
		if _, err := strconv.Atoi(s); s == "" || err == nil || slices.Contains(field.Enum, s) {
			return nil
		}
	}

	return fmt.Errorf("%s is not one of %s, or an integer", describe(value), names)
}

// checkStringList accepts a list of single values, or one string, which is
// how a list arrives from an environment variable ("a b").
func checkStringList(value any) error {
	switch v := value.(type) {
	case string, []string:
		return nil
	case []any:
		for _, item := range v {
			if !isScalar(item) {
				return fmt.Errorf("item %s is not a single value", describe(item))
			}
		}

		return nil
	default:
		return fmt.Errorf("%s is not a list", describe(value))
	}
}

// isScalar reports whether value is a single YAML value, not a list or a
// mapping.
func isScalar(value any) bool {
	switch value.(type) {
	case []any, []string, map[string]any:
		return false
	default:
		return true
	}
}

// describe renders a config value for an error message: strings quoted,
// lists and mappings by their shape.
func describe(value any) string {
	switch v := value.(type) {
	case string:
		return strconv.Quote(v)
	case []any, []string:
		return "a list"
	case map[string]any:
		return "a mapping"
	default:
		return fmt.Sprint(v)
	}
}

// unknownConfigKeys returns a warning for every one of keys (viper.AllKeys,
// the keys set in gooze.yaml plus the schema's defaults) that the schema
// doesn't have, in key order, naming the key it most likely meant.
func unknownConfigKeys(keys []string) []string {
	var warnings []string

	for _, key := range keys {
		if _, ok := config.Lookup(key); ok {
			continue
		}

		msg := fmt.Sprintf("unknown gooze config key %q in %s", key, config.FileName)
		if match := suggestKey(key); match != "" {
			msg += fmt.Sprintf("; did you mean %q?", match)
		}

		warnings = append(warnings, msg)
	}

	slices.Sort(warnings)

	return warnings
}

// maxTypoDistance is the largest edit distance between an unknown key and a
// schema key for the schema key to be suggested.
const maxTypoDistance = 2

// suggestKey returns the schema key an unknown dotted key most likely meant,
// or "" when there is no single good match: the only key with the same last
// name ("parallel" → "run.parallel"), else the only closest key within
// maxTypoDistance edits ("run.paralel" → "run.parallel"). goozels suggests
// the same way in the editor.
func suggestKey(key string) string {
	keys := config.Keys()
	leaf := lastName(key)

	sameLeaf := slices.DeleteFunc(slices.Clone(keys), func(k string) bool { return lastName(k) != leaf })
	if len(sameLeaf) == 1 {
		return sameLeaf[0]
	}

	// closest holds the keys at bestDist, the smallest distance seen so far
	// within reach.
	var closest []string

	bestDist := maxTypoDistance

	for _, k := range keys {
		d := editDistance(key, k)
		if d > bestDist {
			continue
		}

		// Equivalent: d > bestDist was skipped above, so != is <.
		if d < bestDist { //gooze:ignore comparison
			closest, bestDist = closest[:0], d
		}

		closest = append(closest, k)
	}

	if len(closest) != 1 {
		return ""
	}

	return closest[0]
}

func lastName(key string) string {
	return key[strings.LastIndexByte(key, '.')+1:]
}

// editDistance is the Levenshtein distance between a and b, in bytes.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i

		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}

		prev = cur
	}

	return prev[len(b)]
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
		MaxSize:    configInt(config.KeyLogMaxSize),
		MaxBackups: configInt(config.KeyLogMaxBackups),
		MaxAge:     configInt(config.KeyLogMaxAge),
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
	return schemaDefault[string](config.KeyLogFilename)
}

func schemaDefault[T any](key string) T {
	field, _ := config.Lookup(key)
	value, _ := field.Default.(T)

	return value
}
