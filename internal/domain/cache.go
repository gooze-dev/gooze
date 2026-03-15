package domain

import (
	"context"

	"gooze.dev/pkg/gooze/internal/adapter"
	m "gooze.dev/pkg/gooze/internal/model"
)

type Cache interface{
	FilterChangedSources(ctx context.Context, sourceStream <-chan m.Source, reportsPath m.Path, parallel int) (<-chan m.Source, error)
}

type cache struct {
	adapter.ReportStore
	m.Events
}


func NewCache(reportStore adapter.ReportStore, events m.Events) Cache {
	return &cache{
		ReportStore: reportStore,
		Events:      events,
	}
}

// FilterChangedSources takes a stream of sources and filters out those that have changed based on existing reports in the cache.
func (c *cache) FilterChangedSources(ctx context.Context, sourceStream <-chan m.Source, reportsPath m.Path, parallel int) (<-chan m.Source, error) {
	changedSourcesChan := make(chan m.Source, parallel)

	go func() {
		defer close(changedSourcesChan)

		for source := range sourceStream {
			report, err := c.CheckUpdates(ctx, source, reportsPath)
			if err != nil {
				c.Events.Error(fmt.Errorf("Failed to check updates for source %s: %w", source.Origin.FullPath, err))
				continue
			 }
				continue
			}
			if report == nil {
				// If there's no report, we assume the source has changed
				continue
			}
			changedSourcesChan <- source
		}
	}()

	return changedSourcesChan, nil
}