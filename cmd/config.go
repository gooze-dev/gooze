package cmd

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
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

	runParallelConfigKey = "run.parallel"
	excludeConfigKey     = "paths.exclude"

	defaultReportsDir  = ".gooze-reports"
	defaultNoCache     = false
	defaultRunParallel = 1

	envPrefix = "GOOZE"
)

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
	viper.SetDefault(excludeConfigKey, []string{})

	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			return
		}

		return
	}
}
