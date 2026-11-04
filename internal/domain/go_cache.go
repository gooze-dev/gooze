package domain

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"

	pkg "gooze.dev/pkg/gooze/pkg"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// GoCacheMode is where the go command caches the builds of mutants.
type GoCacheMode string

const (
	// GoCacheIsolated caches them in gooze's own cache, which reads through
	// to the shared GOCACHE but never writes to it.
	GoCacheIsolated GoCacheMode = "isolated"
	// GoCacheShared caches them in the shared GOCACHE.
	GoCacheShared GoCacheMode = "shared"
)

// GoCache are a run's Go build cache settings.
type GoCache struct {
	Mode GoCacheMode
	// SizeMB caps gooze's own cache, trimmed at the end of a run.
	SizeMB int
	// Prewarm builds each package once in the shared GOCACHE before its
	// mutants, so its dependencies are cached there.
	Prewarm bool
}

// WithBuildCache isolates the builds of mutants with cache when a run's
// GoCache.Mode is GoCacheIsolated, by setting env to what the cache returns.
func WithBuildCache(cache BuildCache, env BuildEnvironment) WorkflowOption {
	return func(w *workflow) {
		w.buildCache = cache
		w.buildEnv = env
	}
}

// prepareBuildCache prepares the build cache for testing the mutants of
// sources with the go test flags testFlags, and returns the function that
// closes it.
func (w *workflow) prepareBuildCache(ctx context.Context, settings GoCache, testFlags []string, sources pkg.FileSpill[rpt.Source]) (func(), error) {
	if w.buildCache == nil || settings.Mode != GoCacheIsolated {
		return func() {}, nil
	}

	env, err := w.buildCache.Prepare(ctx, settings.SizeMB)
	if err != nil {
		return nil, fmt.Errorf("prepare the go build cache: %w", err)
	}

	if settings.Prewarm && len(env) > 0 {
		if err := w.prewarmBuildCache(ctx, sources, testFlags); err != nil {
			return nil, err
		}
	}

	w.buildEnv.SetBuildEnv(env)

	return func() {
		if err := w.buildCache.Close(context.WithoutCancel(ctx)); err != nil {
			slog.Warn("Failed to trim the go build cache", "error", err)
		}
	}, nil
}

// prewarmBuildCache prewarms the packages of sources that have tests. A failed
// prewarm only slows the run down.
func (w *workflow) prewarmBuildCache(ctx context.Context, sources pkg.FileSpill[rpt.Source], testFlags []string) error {
	dirs, err := packageDirs(sources)
	if err != nil {
		return err
	}

	if len(dirs) == 0 {
		return nil
	}

	if err := w.buildCache.Prewarm(ctx, dirs, testFlags); err != nil {
		slog.Warn("Failed to prewarm the Go build cache", "error", err)
	}

	return nil
}

// packageDirs returns the sorted directories of the sources with tests.
func packageDirs(sources pkg.FileSpill[rpt.Source]) ([]string, error) {
	var dirs []string

	err := sources.Range(func(_ uint64, source rpt.Source) error {
		if source.Test != nil {
			dirs = append(dirs, filepath.Dir(source.Origin.FullPath))
		}

		return nil
	})

	slices.Sort(dirs)

	return slices.Compact(dirs), err
}
