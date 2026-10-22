package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gooze.dev/pkg/gooze/pkg/config"
)

func newInitCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Generate a default gooze.yaml configuration file",
		Long: `Create a gooze.yaml in the current working directory with every key, its
default value and a comment saying what it does, ready to edit. Environment
variables and flags don't change the file. An existing file is never
overwritten.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			targetPath := filepath.Join(deps.Dir, deps.FileName)

			// O_EXCL: never overwrite a file, even one created since the check.
			file, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // a config file is meant to be readable.
			if errors.Is(err, fs.ErrExist) {
				return fmt.Errorf("%s already exists; edit it or remove it first", targetPath)
			}

			if err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}

			_, writeErr := file.Write(config.DefaultFile())
			if err := errors.Join(writeErr, file.Close()); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Wrote the default configuration to %s\n", targetPath)

			return nil
		},
	}
}
