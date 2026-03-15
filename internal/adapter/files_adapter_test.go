package adapter_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/adapter"
	m "gooze.dev/pkg/gooze/internal/model"
	mmocks "gooze.dev/pkg/gooze/internal/model/mocks"
)

const testParallel = 4

func newFilesAdapterWithMock(t *testing.T) (adapter.FilesAdapter, *mmocks.MockEvents) {
	t.Helper()

	events := mmocks.NewMockEvents(t)
	events.EXPECT().StartScanningPaths().Maybe()
	events.EXPECT().ScanningPath(mock.AnythingOfType("model.Path")).Maybe()
	events.EXPECT().FinishScanningPaths().Maybe()
	events.EXPECT().PairFound(mock.AnythingOfType("model.Source")).Maybe()

	return adapter.NewFilesAdapter(events), events
}

func TestFilesAdapter_Get(t *testing.T) {
	t.Run("single source with test pair", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		mainPath := filepath.Join(root, "main.go")
		testPath := filepath.Join(root, "main_test.go")
		copyFile(t, exPath(t, "basic", "main.go"), mainPath)
		copyFile(t, exPath(t, "basic", "main_test.go"), testPath)
		writeFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1)

		src := findByOrigin(sources, mainPath)
		require.NotNilf(t, src, "expected source for %s", mainPath)

		assertOrigin(t, src, mainPath)
		assertTestFile(t, src, testPath)
	})

	t.Run("source without test pair has nil Test", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		// type.go has no type_test.go in basic example
		typePath := filepath.Join(root, "type.go")
		copyFile(t, exPath(t, "basic", "type.go"), typePath)
		writeFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1)

		src := findByOrigin(sources, typePath)
		require.NotNilf(t, src, "expected source for %s", typePath)

		assertOrigin(t, src, typePath)
		assert.Nil(t, src.Test, "Test should be nil when no _test.go companion exists")
	})

	t.Run("multiple sources with mixed pairs", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		// main.go has main_test.go, simple.go has no simple_test.go
		mainPath := filepath.Join(root, "main.go")
		mainTestPath := filepath.Join(root, "main_test.go")
		simplePath := filepath.Join(root, "simple.go")
		copyFile(t, exPath(t, "comparison", "main.go"), mainPath)
		copyFile(t, exPath(t, "comparison", "main_test.go"), mainTestPath)
		copyFile(t, exPath(t, "comparison", "simple.go"), simplePath)
		writeFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 2)

		mainSrc := findByOrigin(sources, mainPath)
		require.NotNilf(t, mainSrc, "expected source for %s", mainPath)
		assertOrigin(t, mainSrc, mainPath)
		assertTestFile(t, mainSrc, mainTestPath)

		simpleSrc := findByOrigin(sources, simplePath)
		require.NotNilf(t, simpleSrc, "expected source for %s", simplePath)
		assertOrigin(t, simpleSrc, simplePath)
		assert.Nil(t, simpleSrc.Test, "simple.go has no test pair")
	})

	t.Run("recursive ./... includes nested files", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		mainPath := filepath.Join(root, "main.go")
		copyFile(t, exPath(t, "basic", "main.go"), mainPath)

		nestedDir := filepath.Join(root, "sub")
		require.NoError(t, os.MkdirAll(nestedDir, 0o755))
		childPath := filepath.Join(nestedDir, "child.go")
		copyFile(t, exPath(t, "nested", "sub", "child.go"), childPath)

		wd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(root))
		t.Cleanup(func() { _ = os.Chdir(wd) })

		ch, err := fa.Get(context.Background(), []m.Path{"./..."}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 2)

		assert.NotNilf(t, findByOrigin(sources, mainPath), "expected top-level main.go")
		assert.NotNilf(t, findByOrigin(sources, childPath), "expected nested child.go")
	})

	t.Run("non-recursive dot skips nested files", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		mainPath := filepath.Join(root, "main.go")
		copyFile(t, exPath(t, "basic", "main.go"), mainPath)

		nestedDir := filepath.Join(root, "sub")
		require.NoError(t, os.MkdirAll(nestedDir, 0o755))
		childPath := filepath.Join(nestedDir, "child.go")
		copyFile(t, exPath(t, "nested", "sub", "child.go"), childPath)

		wd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(root))
		t.Cleanup(func() { _ = os.Chdir(wd) })

		ch, err := fa.Get(context.Background(), []m.Path{"."}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1)

		assert.NotNilf(t, findByOrigin(sources, mainPath), "expected top-level main.go")
		assert.Nilf(t, findByOrigin(sources, childPath), "nested file should be skipped for non-recursive")
	})

	t.Run("test files are not returned as origins", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		mainPath := filepath.Join(root, "main.go")
		testPath := filepath.Join(root, "main_test.go")
		copyFile(t, exPath(t, "basic", "main.go"), mainPath)
		copyFile(t, exPath(t, "basic", "main_test.go"), testPath)
		writeFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)

		for _, src := range sources {
			if src.Origin != nil {
				assert.NotContains(t, string(src.Origin.FullPath), "_test.go",
					"_test.go files must not appear as origins")
			}
		}
	})

	t.Run("non-go files are ignored", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		writeFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")
		writeFile(t, filepath.Join(root, "README.md"), "# hello\n")
		writeFile(t, filepath.Join(root, "config.yaml"), "key: value\n")

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		assert.Empty(t, sources, "non-go files should produce no sources")
	})

	t.Run("broken source files are skipped", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		brokenPath := filepath.Join(root, "broken.go")
		copyFile(t, exPath(t, "invalid", "broken.go"), brokenPath)

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		assert.Empty(t, sources, "broken source files should be skipped")
	})

	t.Run("broken test file yields nil Test", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		sourcePath := filepath.Join(root, "calc.go")
		testPath := filepath.Join(root, "calc_test.go")
		writeFile(t, sourcePath, "package calc\nfunc Sum(a, b int) int { return a + b }\n")
		writeFile(t, testPath, "package calc\nfunc {\n") // intentionally broken

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1)

		src := findByOrigin(sources, sourcePath)
		require.NotNilf(t, src, "expected source for %s", sourcePath)
		assertOrigin(t, src, sourcePath)
		assert.Nil(t, src.Test, "broken test file should result in nil Test")
	})

	t.Run("exclude regex filters matching files", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		keptPath := filepath.Join(root, "keep.go")
		ignoredPath := filepath.Join(root, "generated_skip.go")
		writeFile(t, keptPath, "package main\n")
		writeFile(t, ignoredPath, "package main\n")

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel, "^generated_")
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1)

		assert.NotNilf(t, findByOrigin(sources, keptPath), "keep.go should be present")
		assert.Nilf(t, findByOrigin(sources, ignoredPath), "generated_skip.go should be excluded")
	})

	t.Run("empty roots returns no sources", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)

		ch, err := fa.Get(context.Background(), []m.Path{}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		assert.Empty(t, sources)
	})

	t.Run("returns error for missing root", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)

		ch, err := fa.Get(context.Background(), []m.Path{"/path/does/not/exist"}, testParallel)
		require.NoError(t, err)

		// error comes through the channel — we just get no sources
		sources := drain(t, ch)
		assert.Empty(t, sources)
	})

	t.Run("duplicate roots are de-duplicated", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		mainPath := filepath.Join(root, "main.go")
		copyFile(t, exPath(t, "basic", "main.go"), mainPath)

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root), m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1, "duplicate roots should be de-duplicated")
	})

	t.Run("file path returns single source", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		mainPath := filepath.Join(root, "main.go")
		testPath := filepath.Join(root, "main_test.go")
		copyFile(t, exPath(t, "basic", "main.go"), mainPath)
		copyFile(t, exPath(t, "basic", "main_test.go"), testPath)

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(mainPath)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1)

		src := &sources[0]
		assertOrigin(t, src, mainPath)
		assertTestFile(t, src, testPath)
	})

	t.Run("test file as input yields no sources", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		testPath := filepath.Join(root, "main_test.go")
		copyFile(t, exPath(t, "basic", "main_test.go"), testPath)

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(testPath)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		assert.Empty(t, sources, "test file input should yield no sources")
	})

	t.Run("context cancellation stops scanning", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		mainPath := filepath.Join(root, "main.go")
		copyFile(t, exPath(t, "basic", "main.go"), mainPath)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		ch, err := fa.Get(ctx, []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		assert.Empty(t, sources, "cancelled context should produce no sources")
	})

	t.Run("source has package name set", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		mainPath := filepath.Join(root, "main.go")
		copyFile(t, exPath(t, "basic", "main.go"), mainPath)
		writeFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1)

		require.NotNil(t, sources[0].Package)
		assert.Equal(t, "main", *sources[0].Package)
	})

	t.Run("origin has correct hash", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		mainPath := filepath.Join(root, "main.go")
		copyFile(t, exPath(t, "basic", "main.go"), mainPath)

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1)

		content := readBytes(t, mainPath)
		expectedHash := fmt.Sprintf("%x", sha256.Sum256(content))
		assert.Equal(t, expectedHash, sources[0].Origin.Hash)
	})

	t.Run("origin has short path relative to project root", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		mainPath := filepath.Join(root, "main.go")
		copyFile(t, exPath(t, "basic", "main.go"), mainPath)
		writeFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1)

		src := &sources[0]
		require.NotNil(t, src.Origin)
		assert.Equal(t, m.Path("main.go"), src.Origin.ShortPath, "ShortPath should be relative to project root")
	})

	t.Run("origin short path for nested file", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		nestedDir := filepath.Join(root, "pkg", "utils")
		require.NoError(t, os.MkdirAll(nestedDir, 0o755))
		nestedPath := filepath.Join(nestedDir, "helper.go")
		writeFile(t, nestedPath, "package utils\n")
		writeFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")

		wd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(root))
		t.Cleanup(func() { _ = os.Chdir(wd) })

		// Use ./... to scan recursively
		ch, err := fa.Get(context.Background(), []m.Path{"./..."}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1)

		src := &sources[0]
		require.NotNil(t, src.Origin)
		expectedShort := filepath.Join("pkg", "utils", "helper.go")
		assert.Equal(t, m.Path(expectedShort), src.Origin.ShortPath, "ShortPath should be relative to project root")
	})

	t.Run("test file has short path relative to project root", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		mainPath := filepath.Join(root, "main.go")
		testPath := filepath.Join(root, "main_test.go")
		copyFile(t, exPath(t, "basic", "main.go"), mainPath)
		copyFile(t, exPath(t, "basic", "main_test.go"), testPath)
		writeFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1)

		src := &sources[0]
		require.NotNil(t, src.Test)
		assert.Equal(t, m.Path("main_test.go"), src.Test.ShortPath, "Test ShortPath should be relative to project root")
	})

	t.Run("short path empty when go.mod not found", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		mainPath := filepath.Join(root, "main.go")
		copyFile(t, exPath(t, "basic", "main.go"), mainPath)
		// Intentionally NOT creating go.mod

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1)

		src := &sources[0]
		require.NotNil(t, src.Origin)
		// When go.mod is not found, ShortPath should be empty
		assert.Empty(t, src.Origin.ShortPath, "ShortPath should be empty when go.mod not found")
	})

	t.Run("full path is always absolute", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		mainPath := filepath.Join(root, "main.go")
		copyFile(t, exPath(t, "basic", "main.go"), mainPath)
		writeFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")

		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		sources := drain(t, ch)
		require.Len(t, sources, 1)

		src := &sources[0]
		require.NotNil(t, src.Origin)
		fullPath := string(src.Origin.FullPath)
		assert.True(t, filepath.IsAbs(fullPath), "FullPath should be absolute: %s", fullPath)
		assert.Equal(t, mainPath, fullPath, "FullPath should match absolute input path")
	})
}

func TestFilesAdapter_Get_ChannelCapacity(t *testing.T) {
	t.Run("channel buffer size matches parallel argument", func(t *testing.T) {
		fa, _ := newFilesAdapterWithMock(t)
		root := t.TempDir()

		writeFile(t, filepath.Join(root, "a.go"), "package main\n")

		for _, size := range []int{1, 2, 8, 16} {
			ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, size)
			require.NoError(t, err)

			assert.Equal(t, size, cap(ch), "channel capacity should equal parallel=%d", size)
			drain(t, ch)
		}
	})
}

func TestFilesAdapter_Get_EmitsEvents(t *testing.T) {
	t.Run("emits scan events for each root", func(t *testing.T) {
		events := mmocks.NewMockEvents(t)

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "main.go"), "package main\n")

		absRoot, err := filepath.Abs(root)
		require.NoError(t, err)

		events.EXPECT().StartScanningPaths().Times(1)
		events.EXPECT().ScanningPath(m.Path(absRoot)).Times(1)
		events.EXPECT().FinishScanningPaths().Times(1)
		events.EXPECT().PairFound(mock.MatchedBy(func(source m.Source) bool {
			return source.Origin != nil && strings.HasSuffix(string(source.Origin.FullPath), "main.go")
		})).Times(1)

		fa := adapter.NewFilesAdapter(events)
		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root)}, testParallel)
		require.NoError(t, err)

		drain(t, ch)

		events.AssertExpectations(t)
	})

	t.Run("emits scan events for multiple roots", func(t *testing.T) {
		events := mmocks.NewMockEvents(t)

		root1 := t.TempDir()
		root2 := t.TempDir()
		writeFile(t, filepath.Join(root1, "a.go"), "package a\n")
		writeFile(t, filepath.Join(root2, "b.go"), "package b\n")

		absRoot1, err := filepath.Abs(root1)
		require.NoError(t, err)
		absRoot2, err := filepath.Abs(root2)
		require.NoError(t, err)

		events.EXPECT().StartScanningPaths().Times(1)
		events.EXPECT().ScanningPath(m.Path(absRoot1)).Times(1)
		events.EXPECT().ScanningPath(m.Path(absRoot2)).Times(1)
		events.EXPECT().FinishScanningPaths().Times(1)
		events.EXPECT().PairFound(mock.MatchedBy(func(source m.Source) bool {
			return source.Origin != nil && (strings.HasSuffix(string(source.Origin.FullPath), "a.go") || strings.HasSuffix(string(source.Origin.FullPath), "b.go"))
		})).Times(2)

		fa := adapter.NewFilesAdapter(events)
		ch, err := fa.Get(context.Background(), []m.Path{m.Path(root1), m.Path(root2)}, testParallel)
		require.NoError(t, err)

		drain(t, ch)

		events.AssertExpectations(t)
	})

	t.Run("no events emitted for empty roots", func(t *testing.T) {
		events := mmocks.NewMockEvents(t)

		fa := adapter.NewFilesAdapter(events)
		ch, err := fa.Get(context.Background(), []m.Path{}, testParallel)
		require.NoError(t, err)

		drain(t, ch)

		events.AssertNotCalled(t, "StartScanningPaths")
		events.AssertNotCalled(t, "ScanningPath", mock.Anything)
		events.AssertNotCalled(t, "FinishScanningPaths")
	})
}

// --- test helpers ---

func drain(t *testing.T, ch <-chan m.Source) []m.Source {
	t.Helper()

	var sources []m.Source
	for src := range ch {
		sources = append(sources, src)
	}

	return sources
}

func findByOrigin(sources []m.Source, path string) *m.Source {
	for i := range sources {
		if sources[i].Origin != nil && string(sources[i].Origin.FullPath) == path {
			return &sources[i]
		}
	}

	return nil
}

func assertOrigin(t *testing.T, src *m.Source, expectedPath string) {
	t.Helper()

	require.NotNil(t, src.Origin, "Origin should not be nil")
	assert.Equal(t, m.Path(expectedPath), src.Origin.FullPath)
	assert.NotEmpty(t, src.Origin.Hash, "Origin.Hash should be set")
}

func assertTestFile(t *testing.T, src *m.Source, expectedPath string) {
	t.Helper()

	require.NotNilf(t, src.Test, "Test should not be nil for %s", expectedPath)
	assert.Equal(t, m.Path(expectedPath), src.Test.FullPath)
	assert.NotEmpty(t, src.Test.Hash, "Test.Hash should be set")
}

func exPath(t *testing.T, elem ...string) string {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err)

	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	parts := append([]string{repoRoot, "examples"}, elem...)

	return filepath.Join(parts...)
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()

	content := readBytes(t, src)
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
	require.NoError(t, os.WriteFile(dst, content, 0o644))
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	return content
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}