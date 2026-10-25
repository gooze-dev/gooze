package gocache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gooze.dev/pkg/gooze/internal/domain"
)

// minGoMinor is the first Go 1.x release with GOCACHEPROG.
const minGoMinor = 24

var goMinor = regexp.MustCompile(`^(?:devel )?go1\.(\d+)`)

// Cache is the domain.BuildCache that isolates the builds of mutants in
// gooze's own cache, through GOCACHEPROG.
type Cache struct {
	setEnv func(env []string)
	layer  *layer
	limit  int64
}

// New returns a Cache that hands the go command's environment to setEnv.
func New(setEnv func(env []string)) *Cache {
	return &Cache{setEnv: setEnv}
}

type goEnv struct {
	GOVERSION, GOCACHE, GOCACHEPROG string
}

// Prepare prewarms the shared cache with the packages in pkgDirs when the
// settings ask for it, then sets GOCACHEPROG to gooze's server. Without
// GOCACHEPROG support, or with GOCACHEPROG already set, the mutants use the
// shared cache.
func (c *Cache) Prepare(ctx context.Context, settings domain.GoCache, pkgDirs []string) error {
	env, err := readGoEnv(ctx)
	if err != nil {
		return err
	}

	if reason := fallbackReason(env); reason != "" {
		slog.Info("Building mutants in the shared Go build cache", "reason", reason)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	userCache, err := os.UserCacheDir()
	if err != nil {
		return err
	}

	if settings.Prewarm && len(pkgDirs) > 0 {
		prewarm(ctx, pkgDirs)
	}

	c.layer = &layer{dir: filepath.Join(userCache, "gooze", "go-build")}
	c.limit = int64(settings.SizeMB) << 20
	c.setEnv([]string{"GOCACHEPROG=" + progCommand(exe, env.GOCACHE, c.layer.dir)})

	return nil
}

// Close trims gooze's cache to its size.
func (c *Cache) Close(context.Context) error {
	if c.layer == nil {
		return nil
	}

	return c.layer.trim(c.limit, time.Now())
}

func readGoEnv(ctx context.Context) (goEnv, error) {
	out, err := exec.CommandContext(ctx, "go", "env", "-json", "GOVERSION", "GOCACHE", "GOCACHEPROG").Output()
	if err != nil {
		return goEnv{}, fmt.Errorf("go env: %w", err)
	}

	var env goEnv

	return env, json.Unmarshal(out, &env)
}

// fallbackReason says why mutants can't use gooze's cache, or is empty.
func fallbackReason(env goEnv) string {
	switch {
	case env.GOCACHEPROG != "":
		return "GOCACHEPROG is already set"
	case !supportsCacheProg(env.GOVERSION):
		return "GOCACHEPROG needs Go 1.24 or later, not " + env.GOVERSION
	default:
		return ""
	}
}

func supportsCacheProg(version string) bool {
	match := goMinor.FindStringSubmatch(version)
	if match == nil {
		return false
	}

	minor, err := strconv.Atoi(match[1])

	return err == nil && minor >= minGoMinor
}

// prewarm builds the tests of the packages in dirs in the shared cache. A
// failure only costs the speedup, so it is logged.
func prewarm(ctx context.Context, dirs []string) {
	cmd := exec.CommandContext(ctx, "go", append([]string{"test", "-count=1", "-vet=off", "-run", "^$"}, dirs...)...)
	cmd.Dir = dirs[0]

	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Warn("Failed to prewarm the Go build cache", "error", err, "output", string(out))
	}
}

// progCommand returns the GOCACHEPROG value that runs gooze's server, its
// arguments quoted as the go command splits them.
func progCommand(exe, shared, layerDir string) string {
	quote := func(s string) string {
		if strings.Contains(s, `"`) {
			return "'" + s + "'"
		}

		return `"` + s + `"`
	}

	return quote(exe) + " __cacheprog --shared " + quote(shared) + " --layer " + quote(layerDir)
}
