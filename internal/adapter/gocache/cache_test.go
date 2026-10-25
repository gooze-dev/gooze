package gocache

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

		if err := Serve(os.Stdin, os.Stdout, *shared, *layerDir); err != nil {
			os.Exit(1)
		}

		os.Exit(0)
	}

	code := m.Run()
	_ = os.RemoveAll(template.dir)

	os.Exit(code)
}

// template is a GOCACHE holding the standard library's test builds, which
// newModule copies so that each test starts from a warm cache.
var template struct {
	once sync.Once
	dir  string
	err  error
}

func warmTemplate(t *testing.T) string {
	t.Helper()

	template.once.Do(func() {
		if template.dir, template.err = os.MkdirTemp("", "gocache-template"); template.err != nil {
			return
		}

		mod := filepath.Join(template.dir, "mod")
		_ = os.MkdirAll(mod, 0o755)
		_ = os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module warm\n\ngo 1.24\n"), 0o600)
		_ = os.WriteFile(filepath.Join(mod, "warm_test.go"), []byte("package warm\n\nimport \"testing\"\n\nfunc TestWarm(t *testing.T) {}\n"), 0o600)

		cmd := exec.Command("go", "test", "-count=1", ".")
		cmd.Dir = mod
		cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(template.dir, "go-build"), "GOCACHEPROG=", "GOFLAGS=-buildvcs=false")

		if out, err := cmd.CombinedOutput(); err != nil {
			template.err = fmt.Errorf("%w: %s", err, out)
		}
	})
	require.NoError(t, template.err)

	return filepath.Join(template.dir, "go-build")
}

// module is a module whose main package imports its lib package, in a
// temporary GOCACHE and user cache directory.
type module struct {
	dir, shared, layer string
}

func newModule(t *testing.T) module {
	t.Helper()

	tmp := t.TempDir()
	mod := module{dir: filepath.Join(tmp, "calc"), shared: filepath.Join(tmp, "go-build"), layer: filepath.Join(tmp, "xdg", "gooze", "go-build")}
	require.NoError(t, os.CopyFS(mod.shared, os.DirFS(warmTemplate(t))))
	t.Setenv("GOCACHE", mod.shared)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "xdg"))
	t.Setenv("GOCACHEPROG", "")
	t.Setenv("GOFLAGS", "-buildvcs=false")

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

func files(t *testing.T, dir string) []string {
	t.Helper()

	var paths []string

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			paths = append(paths, path)
		}

		return nil
	})

	return paths
}

func prepare(t *testing.T, settings domain.GoCache, dirs ...string) (*Cache, []string) {
	t.Helper()

	var env []string

	cache := New(func(e []string) { env = e })
	require.NoError(t, cache.Prepare(context.Background(), settings, dirs))

	return cache, env
}

func TestCache_IsolatesBuilds(t *testing.T) {
	mod := newModule(t)
	mod.goCmd(t, nil, "build", "./lib")
	shared := files(t, mod.shared)

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

	second := mod.goCmd(t, env, "build", "-x", "-o", bin, ".")
	assert.NotContains(t, second, "-p main ", "main is read from the layer")

	assert.NotEmpty(t, files(t, mod.layer))
	assert.Equal(t, shared, files(t, mod.shared))
}

func TestCache_LeavesSharedCacheAlone(t *testing.T) {
	mod := newModule(t)
	mod.goCmd(t, nil, "build", "./lib")
	shared := files(t, mod.shared)

	_, env := prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated})
	mod.goCmd(t, env, "test", "-count=1", "./...")
	mod.goCmd(t, env, "build", "-o", filepath.Join(t.TempDir(), "calc"), ".")

	assert.Equal(t, shared, files(t, mod.shared))
}

func TestCache_Prewarm(t *testing.T) {
	mod := newModule(t)
	mod.goCmd(t, nil, "build", "./lib")
	shared := files(t, mod.shared)

	_, env := prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated, Prewarm: true}, filepath.Join(mod.dir, "lib"), mod.dir)
	assert.Len(t, env, 1)
	assert.Greater(t, len(files(t, mod.shared)), len(shared), "the packages' test builds go to the shared cache")
	assert.Empty(t, files(t, mod.layer))
}

func TestCache_PrewarmFailureIsNotFatal(t *testing.T) {
	mod := newModule(t)
	writeTestFile(t, filepath.Join(mod.dir, "lib", "broken.go"), "package lib\n\nfunc Broken() int { return \"\" }\n")

	_, env := prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated, Prewarm: true}, filepath.Join(mod.dir, "lib"))
	assert.Len(t, env, 1)
}

func TestCache_NoPrewarmWithoutPackages(t *testing.T) {
	mod := newModule(t)
	shared := files(t, mod.shared)

	_, env := prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated, Prewarm: true})
	assert.Len(t, env, 1)
	assert.Equal(t, shared, files(t, mod.shared))
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
	shared := files(t, mod.shared)
	t.Setenv("GOCACHEPROG", "/no/such/cacheprog")

	cache, env := prepare(t, domain.GoCache{Mode: domain.GoCacheIsolated, Prewarm: true}, mod.dir)
	assert.Nil(t, env)
	assert.Equal(t, shared, files(t, mod.shared), "no prewarm")

	require.NoError(t, cache.Close(context.Background()))
	assert.FileExists(t, path, "no trim")
}

func TestCache_GoEnvFails(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := New(func([]string) { t.Error("env set") }).Prepare(context.Background(), domain.GoCache{}, nil)
	require.Error(t, err)
}
