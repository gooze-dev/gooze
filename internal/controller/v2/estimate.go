package v2

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gooze.dev/pkg/gooze/internal/controller/v2/ui"
	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

func Estimate(cmd *cobra.Command, args []string) error {
	u := ui.NewUI(cmd, ui.IsTTY(cmd.OutOrStdout()))

	done := make(chan struct{})
	go func() {
		defer close(done)
		u.ShowEstimateProgress(cmd.Context(), events)
	}()

	err := workflow.Estimate(cmd.Context(), events, getOptions(cmd))
	events.Close()
	<-done
	return err
}

func getOptions(cmd *cobra.Command) domain.EstimateOptions {
	paths := parsePaths(cmd.Flags().Args())
	excludePaths := viper.GetStringSlice("exclude")
	useCache := !viper.GetBool("no-cache")
	reportsPath := m.Path(viper.GetString("output"))

	return domain.EstimateOptions{
		Paths:        paths,
		ExcludePaths: excludePaths,
		UseCache:     useCache,
		ReportsPath:  reportsPath,
	}
}
