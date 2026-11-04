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

// prepareBuildCache prepares the build cache of a run or an estimate and sets
// the environment of the go commands that build to use it. It returns that
// environment, empty when they use the shared cache, and the function that
// closes the cache.
func (w *workflow) prepareBuildCache(ctx context.Context, settings GoCache) ([]string, func(), error) {
	if w.buildCache == nil || settings.Mode != GoCacheIsolated {
		return nil, func() {}, nil
	}

	env, err := w.buildCache.Prepare(ctx, settings.SizeMB)
	if err != nil {
		return nil, nil, fmt.Errorf("prepare the go build cache: %w", err)
	}

	w.buildEnv.SetBuildEnv(env)

	return env, func() {
		if err := w.buildCache.Close(context.WithoutCancel(ctx)); err != nil {
			slog.Warn("Failed to trim the go build cache", "error", err)
		}
	}, nil
}

// prewarmBuildCache prewarms the packages of sources that have tests when
// settings ask for it and the cache is isolated (env is set). A failed prewarm
// only slows the run down.
func (w *workflow) prewarmBuildCache(ctx context.Context, settings GoCache, env []string, sources pkg.FileSpill[rpt.Source]) error {
	if !settings.Prewarm || len(env) == 0 {
		return nil
	}

	dirs, err := packageDirs(sources)
	if err != nil {
		return err
	}

	if len(dirs) == 0 {
		return nil
	}

	if err := w.buildCache.Prewarm(ctx, dirs); err != nil {
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
