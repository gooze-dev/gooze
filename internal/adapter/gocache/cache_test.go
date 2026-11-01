package gocache

import (
	"context"
	"flag"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/adapter/gocache/cacheprog"
	"gooze.dev/pkg/gooze/internal/domain"
)

// TestMain serves GOCACHEPROG when the go command starts the test binary as
// the program Prepare sets.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "__cacheprog" {
		flags := flag.NewFlagSet("cacheprog", flag.ExitOnError)
		shared := flags.String("shared", "", "")
		layerDir := flags.String("layer", "", "")
		_ = flags.Parse(os.Args[2:])

		if err := cacheprog.Serve(os.Stdin, os.Stdout, *shared, *layerDir); err != nil {
			os.Exit(1)
		}

		os.Exit(0)
	}

	os.Exit(m.Run())
}

// module is a module whose main package imports its lib package. Its builds
// read the developer's GOCACHE, which already holds the standard library;
// its directory makes its own packages' cache entries unique.
type module struct {
	dir, shared, layer string
}

func newModule(t *testing.T) module {
	t.Helper()

	shared, err := exec.Command("go", "env", "GOCACHE").Output()
	require.NoError(t, err)

	tmp := t.TempDir()
	t.Setenv("GOCACHE", strings.TrimSpace(string(shared)))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "xdg"))
	t.Setenv("GOCACHEPROG", "")
	t.Setenv("GOFLAGS", "-buildvcs=false")

	mod := module{dir: filepath.Join(tmp, "calc"), shared: strings.TrimSpace(string(shared)), layer: filepath.Join(tmp, "xdg", "gooze", "go-build")}
	writeTestFile(t, filepath.Join(mod.dir, "go.mod"), "module calc\n\ngo 1.24\n")
	writeTestFile(t, filepath.Join(mod.dir, "lib", "lib.go"), "package lib\n\nfunc Add(a, b int) int { return a + b }\n")
	writeTestFile(t, filepath.Join(mod.dir, "lib", "lib_test.go"), "package lib\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {}\n")
	writeTestFile(t, filepath.Join(mod.dir, "main.go"), "package main\n\nimport \"calc/lib\"\n\nfunc main() { println(lib.Add(2, 3)) }\n")

	return mod
}

// goCmd runs go in the module with env added and returns its output.
func (mod module) goCmd(t *testing.T, env []string, args ...string) string {
	t.Helper()

	cmd := exec.Command("go", args...)
	cmd.Dir = mod.dir
	cmd.Env = append(os.Environ(), env...)

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go %v\n%s", args, out)

	return string(out)
}

// build builds the module's main package with env and returns the -x output.
func (mod module) build(t *testing.T, env []string) string {
	t.Helper()

	return mod.goCmd(t, env, "build", "-x", "-o", filepath.Join(t.TempDir(), "calc"), ".")
}

// layerFiles returns the layer's files, relative to it.
func (mod module) layerFiles(t *testing.T) []string {
	t.Helper()

	var paths []string

	_ = filepath.WalkDir(mod.layer, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(mod.layer, path)
			paths = append(paths, rel)
		}

		return nil
	})

	return paths
}

// assertNotShared checks that no entry of the layer was written to the shared
// cache too. Outputs are named by their content, so both may hold one.
func (mod module) assertNotShared(t *testing.T) {
	t.Helper()

	files := mod.layerFiles(t)
	require.NotEmpty(t, files)

	for _, file := range files {
		if strings.HasSuffix(file, "-a") {
			assert.NoFileExists(t, filepath.Join(mod.shared, file))
		}
	}
}

func prepare(t *testing.T, settings domain.GoCache, dirs ...string) (*Cache, []string) {
	t.Helper()

	cache := New()

	env, err := cache.Prepare(context.Background(), settings.SizeMB)
	require.NoError(t, err)

	if settings.Prewarm {
		require.NoError(t, cache.Prewarm(context.Background(), dirs))
	}

	return cache, env
}

func TestCache_IsolatesBuilds(t *testing.T) {
	mod := newModule(t)
	mod.goCmd(t, nil, "build", "./lib")

	_, env := prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated})
	require.Len(t, env, 1)
	assert.True(t, strings.HasPrefix(env[0], "GOCACHEPROG="), env[0])

	bin := filepath.Join(t.TempDir(), "calc")
	first := mod.goCmd(t, env, "build", "-x", "-o", bin, ".")
	assert.NotContains(t, first, "-p calc/lib ", "lib is read from the shared cache")
	assert.Contains(t, first, "-p main ")

	out, err := exec.Command(bin).CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, "5\n", string(out))

	assert.NotContains(t, mod.build(t, env), "-p main ", "main is read from the layer")
	mod.assertNotShared(t)
}

func TestCache_LeavesSharedCacheAlone(t *testing.T) {
	mod := newModule(t)

	_, env := prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated})
	mod.goCmd(t, env, "test", "-count=1", "./...")
	mod.build(t, env)

	mod.assertNotShared(t)
}

func TestCache_Prewarm(t *testing.T) {
	mod := newModule(t)

	_, env := prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated, Prewarm: true}, filepath.Join(mod.dir, "lib"), mod.dir)
	require.Len(t, env, 1)
	assert.Empty(t, mod.layerFiles(t))

	assert.NotContains(t, mod.build(t, env), "-p calc/lib ", "lib was built in the shared cache")
}

func TestCache_NoPrewarm(t *testing.T) {
	mod := newModule(t)

	_, env := prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated}, filepath.Join(mod.dir, "lib"), mod.dir)
	assert.Contains(t, mod.build(t, env), "-p calc/lib ")
}

// oldLayerFile writes a file of 600 KB to the layer, last used a day ago.
func oldLayerFile(t *testing.T, mod module) string {
	t.Helper()

	path := filepath.Join(mod.layer, "aa", "aa-d")
	writeTestFile(t, path, string(make([]byte, 600<<10)))

	old := time.Now().Add(-24 * time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))

	return path
}

func TestCache_CloseTrimsToSize(t *testing.T) {
	mod := newModule(t)
	path := oldLayerFile(t, mod)

	cache, _ := prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated, SizeMB: 1})
	require.NoError(t, cache.Close(context.Background()))
	assert.FileExists(t, path, "600 KB fit in 1 MB")

	cache, _ = prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated})
	require.NoError(t, cache.Close(context.Background()))
	assert.NoFileExists(t, path)
}

func TestCache_FallsBackWhenGOCACHEPROGIsSet(t *testing.T) {
	mod := newModule(t)
	path := oldLayerFile(t, mod)
	t.Setenv("GOCACHEPROG", "/no/such/cacheprog")

	cache, env := prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated})
	assert.Nil(t, env)

	require.NoError(t, cache.Close(context.Background()))
	assert.FileExists(t, path, "no trim")
}

func TestCache_GoEnvFails(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	env, err := New().Prepare(context.Background(), 0)
	require.Error(t, err)
	assert.Empty(t, env)
}

func TestCache_PrewarmOnePackage(t *testing.T) {
	mod := newModule(t)

	_, env := prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated, Prewarm: true}, mod.dir)
	assert.NotContains(t, mod.build(t, env), "-p calc/lib ", "lib was built in the shared cache")
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestCache_PrewarmReportsAFailure(t *testing.T) {
	mod := newModule(t)
	cache, _ := prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated})

	lib := filepath.Join(mod.dir, "lib")
	require.NoError(t, cache.Prewarm(context.Background(), []string{lib}))

	writeTestFile(t, filepath.Join(lib, "broken.go"), "package lib\n\nfunc Broken() int { return \"\" }\n")
	require.ErrorContains(t, cache.Prewarm(context.Background(), []string{lib}), "broken.go")
}
