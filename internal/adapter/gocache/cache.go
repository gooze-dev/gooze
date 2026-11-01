// Package gocache keeps the builds of mutants out of the user's Go build cache
// by running them through the cacheprog server.
package gocache

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gooze.dev/pkg/gooze/internal/adapter/gocache/cacheprog"
	"gooze.dev/pkg/gooze/internal/domain"
)

var _ domain.BuildCache = (*Cache)(nil)

// Cache isolates the builds of mutants in gooze's own cache, through
// GOCACHEPROG.
type Cache struct {
	layer *cacheprog.Layer
	limit int64
}

// New returns a Cache.
func New() *Cache {
	return &Cache{}
}

// Prepare returns the GOCACHEPROG setting of gooze's server. Without
// GOCACHEPROG support, or with GOCACHEPROG already set, it returns none and
// the mutants use the shared cache.
func (c *Cache) Prepare(ctx context.Context, sizeMB int) ([]string, error) {
	env, err := readGoEnv(ctx)
	if err != nil {
		return nil, err
	}

	if reason := fallbackReason(env); reason != "" {
		slog.Info("Building mutants in the shared Go build cache", "reason", reason)
		return nil, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}

	userCache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}

	c.layer = &cacheprog.Layer{Dir: filepath.Join(userCache, "gooze", "go-build")}
	c.limit = int64(sizeMB) << 20

	return []string{"GOCACHEPROG=" + progCommand(exe, env.GOCACHE, c.layer.Dir)}, nil
}

// Prewarm builds the tests of the packages in pkgDirs in the shared cache.
func (c *Cache) Prewarm(ctx context.Context, pkgDirs []string) error {
	cmd := exec.CommandContext(ctx, "go", append([]string{"test", "-count=1", "-vet=off", "-run", "^$"}, pkgDirs...)...) //nolint:gosec // pkgDirs are the run's package directories.
	cmd.Dir = pkgDirs[0]

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go test: %w: %s", err, out)
	}

	return nil
}

// Close trims gooze's cache to its size.
func (c *Cache) Close(context.Context) error {
	if c.layer == nil {
		return nil
	}

	return c.layer.Trim(c.limit, time.Now())
}
