package adapter

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalSourceFSAdapter_Walk(t *testing.T) {
	t.Run("non recursive skips nested files", func(t *testing.T) {
		adapter := NewLocalSourceFSAdapter()

		root := t.TempDir()
		writeTestFile(t, filepath.Join(root, "main.go"), "package main\n")

		nestedDir := filepath.Join(root, "nested")
		mustMkdir(t, nestedDir)
		writeTestFile(t, filepath.Join(nestedDir, "child.go"), "package nested\n")

		var visited []string
		err := adapter.Walk(m.Path(root), false, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			visited = append(visited, path)
			return nil
		})
		require.NoError(t, err)

		for _, forbidden := range []string{nestedDir, filepath.Join(nestedDir, "child.go")} {
			assert.Falsef(t, containsPath(visited, forbidden), "Walk() unexpectedly visited %s when recursive is false", forbidden)
		}

		assert.True(t, containsPath(visited, filepath.Join(root, "main.go")), "Walk() did not visit top-level file")
	})

	t.Run("recursive visits nested files", func(t *testing.T) {
		adapter := NewLocalSourceFSAdapter()

		root := t.TempDir()
		writeTestFile(t, filepath.Join(root, "main.go"), "package main\n")

		nestedDir := filepath.Join(root, "nested")
		mustMkdir(t, nestedDir)
		child := filepath.Join(nestedDir, "child.go")
		writeTestFile(t, child, "package nested\n")

		var visited []string
		err := adapter.Walk(m.Path(root), true, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			visited = append(visited, path)
			return nil
		})
		require.NoError(t, err)

		assert.True(t, containsPath(visited, child), "Walk() did not visit nested file when recursive")
	})
}

func TestLocalSourceFSAdapter_ReadFile(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	content := "package main\n" + "func main() {}\n"
	writeTestFile(t, path, content)

	got, err := adapter.ReadFile(m.Path(path))
	require.NoError(t, err)

	assert.Equal(t, content, string(got))
}

func TestLocalSourceFSAdapter_HashFile(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	content := []byte("package main\nfunc main() {}\n")
	writeTestBytes(t, path, content)

	expected := fmt.Sprintf("%x", sha256.Sum256(content))

	hash, err := adapter.HashFile(m.Path(path))
	require.NoError(t, err)

	assert.Equal(t, expected, hash)
}

func TestLocalSourceFSAdapter_DetectTestFile(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	root := t.TempDir()
	source := filepath.Join(root, "calc.go")
	testFile := filepath.Join(root, "calc_test.go")
	writeTestFile(t, source, "package calc\n")
	writeTestFile(t, testFile, "package calc\n")

	got, err := adapter.DetectTestFile(m.Path(source))
	require.NoError(t, err)

	assert.Equal(t, m.Path(testFile), got)

	t.Run("returns empty path when test file missing", func(t *testing.T) {
		missingSrc := filepath.Join(root, "other.go")
		writeTestFile(t, missingSrc, "package main\n")

		got, err := adapter.DetectTestFile(m.Path(missingSrc))
		require.NoError(t, err)

		assert.Empty(t, got)
	})
}

func TestLocalSourceFSAdapter_FileInfo(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	writeTestFile(t, path, "package main\n")

	info, err := adapter.FileInfo(m.Path(path))
	require.NoError(t, err)

	assert.False(t, info.IsDir(), "FileInfo() reported file as directory")

	dirInfo, err := adapter.FileInfo(m.Path(root))
	require.NoError(t, err)
	assert.True(t, dirInfo.IsDir(), "FileInfo() reported directory as file")
}

func TestLocalSourceFSAdapter_FindProjectRoot(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	root := t.TempDir()
	goModDir := filepath.Join(root, "project")
	mustMkdir(t, goModDir)
	goModPath := filepath.Join(goModDir, "go.mod")
	writeTestFile(t, goModPath, "module example.com/project\n")

	subDir := filepath.Join(goModDir, "sub", "pkg")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		require.NoError(t, err)
	}

	got, err := adapter.FindProjectRoot(m.Path(filepath.Join(subDir, "file.go")))
	require.NoError(t, err)

	assert.Equal(t, m.Path(goModDir), got)
}

func TestLocalSourceFSAdapter_CreateTempDirAndRemoveAll(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	tmp, err := adapter.CreateTempDir("gooze-test-*")
	require.NoError(t, err)

	if fi, err := os.Stat(string(tmp)); err != nil || !fi.IsDir() {
		require.NoError(t, err)
		assert.True(t, fi.IsDir())
	}

	filePath := filepath.Join(string(tmp), "file.go")
	writeTestFile(t, filePath, "package main\n")

	if err := adapter.RemoveAll(tmp); err != nil {
		require.NoError(t, err)
	}

	_, err = os.Stat(string(tmp))
	assert.True(t, os.IsNotExist(err))
}

func TestLocalSourceFSAdapter_CopyDirAndWriteFile(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	src := t.TempDir()
	dst := t.TempDir()

	subDir := filepath.Join(src, "sub")
	mustMkdir(t, subDir)
	filePath := filepath.Join(subDir, "main.go")
	writeTestFile(t, filePath, "package main\n")

	// Additional file written via adapter.WriteFile
	extraFile := filepath.Join(src, "extra.go")
	if err := adapter.WriteFile(m.Path(extraFile), []byte("package extra\n"), 0o644); err != nil {
		require.NoError(t, err)
	}

	if err := adapter.CopyDir(m.Path(src), m.Path(dst)); err != nil {
		require.NoError(t, err)
	}

	// Check that files exist in destination
	if _, err := os.Stat(filepath.Join(dst, "sub", "main.go")); err != nil {
		require.NoError(t, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "extra.go")); err != nil {
		require.NoError(t, err)
	}
}

func TestLocalSourceFSAdapter_PathHelpers(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	base := m.Path("/tmp/project")
	target := m.Path("/tmp/project/sub/dir/file.go")

	rel, err := adapter.RelPath(base, target)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join("sub", "dir", "file.go"), string(rel))

	joined := adapter.JoinPath("/tmp", "project", "sub", "file.go")
	assert.Equal(t, filepath.Join("/tmp", "project", "sub", "file.go"), string(joined))
}

func TestLocalSourceFSAdapter_Get(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	t.Run("dot selects current directory non-recursive", func(t *testing.T) {
		root := t.TempDir()
		basicDir := examplePath(t, "basic")
		mainPath := filepath.Join(root, "main.go")
		testPath := filepath.Join(root, "main_test.go")
		copyExampleFile(t, filepath.Join(basicDir, "main.go"), mainPath)
		copyExampleFile(t, filepath.Join(basicDir, "main_test.go"), testPath)
		mainContent := readFileBytes(t, mainPath)
		testContent := readFileBytes(t, testPath)

		nestedDir := filepath.Join(root, "nested")
		mustMkdir(t, nestedDir)
		nestedPath := filepath.Join(nestedDir, "child.go")
		copyExampleFile(t, filepath.Join(examplePath(t, "nested", "sub"), "child.go"), nestedPath)

		wd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(root))
		t.Cleanup(func() { _ = os.Chdir(wd) })

		sources, err := adapter.Get([]m.Path{"."})
		require.NoError(t, err)

		require.Len(t, sources, 1)

		source := findSourceV2ByOrigin(sources, mainPath)
		require.NotNilf(t, source, "Get() did not include %s", mainPath)

		assertSourceV2(t, source, mainPath, mainContent, "main", testPath, testContent)

		assert.Nil(t, findSourceV2ByOrigin(sources, nestedPath), "Get() unexpectedly included nested file for '.'")

		assert.Nil(t, findSourceV2ByOrigin(sources, testPath), "Get() should not include test files as origins")
	})

	t.Run("tilde expands home directory", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		mainPath := filepath.Join(home, "home.go")
		copyExampleFile(t, filepath.Join(examplePath(t, "basic"), "main.go"), mainPath)
		mainContent := readFileBytes(t, mainPath)

		sources, err := adapter.Get([]m.Path{"~"})
		require.NoError(t, err)

		source := findSourceV2ByOrigin(sources, mainPath)
		require.NotNilf(t, source, "Get() did not include %s", mainPath)

		assertSourceV2(t, source, mainPath, mainContent, "main", "", nil)
	})

	t.Run("parent directory path resolves", func(t *testing.T) {
		root := t.TempDir()
		parentPath := filepath.Join(root, "main.go")
		copyExampleFile(t, filepath.Join(examplePath(t, "basic"), "main.go"), parentPath)
		parentContent := readFileBytes(t, parentPath)

		childDir := filepath.Join(root, "child")
		mustMkdir(t, childDir)

		wd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(childDir))
		t.Cleanup(func() { _ = os.Chdir(wd) })

		sources, err := adapter.Get([]m.Path{"./../"})
		require.NoError(t, err)

		source := findSourceV2ByOrigin(sources, parentPath)
		require.NotNilf(t, source, "Get() did not include %s", parentPath)

		assertSourceV2(t, source, parentPath, parentContent, "main", "", nil)
	})

	t.Run("go style recursive path includes nested", func(t *testing.T) {
		root := t.TempDir()
		mainPath := filepath.Join(root, "main.go")
		copyExampleFile(t, filepath.Join(examplePath(t, "basic"), "main.go"), mainPath)
		mainContent := readFileBytes(t, mainPath)

		nestedDir := filepath.Join(root, "nested")
		mustMkdir(t, nestedDir)
		nestedPath := filepath.Join(nestedDir, "child.go")
		copyExampleFile(t, filepath.Join(examplePath(t, "nested", "sub"), "child.go"), nestedPath)
		nestedContent := readFileBytes(t, nestedPath)

		wd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(root))
		t.Cleanup(func() { _ = os.Chdir(wd) })

		sources, err := adapter.Get([]m.Path{"./..."})
		require.NoError(t, err)
		mainSource := findSourceV2ByOrigin(sources, mainPath)
		require.NotNilf(t, mainSource, "Get() did not include %s", mainPath)
		assertSourceV2(t, mainSource, mainPath, mainContent, "main", "", nil)

		nestedSource := findSourceV2ByOrigin(sources, nestedPath)
		require.NotNil(t, nestedSource, "Get() did not include nested file for ./...")

		assertSourceV2(t, nestedSource, nestedPath, nestedContent, "sub", "", nil)
	})

	t.Run("returns error for missing root", func(t *testing.T) {
		_, err := adapter.Get([]m.Path{"/path/does/not/exist"})
		assert.Error(t, err)
	})

	t.Run("file path returns single source", func(t *testing.T) {
		root := t.TempDir()
		mainPath := filepath.Join(root, "main.go")
		testPath := filepath.Join(root, "main_test.go")
		copyExampleFile(t, filepath.Join(examplePath(t, "basic"), "main.go"), mainPath)
		copyExampleFile(t, filepath.Join(examplePath(t, "basic"), "main_test.go"), testPath)
		mainContent := readFileBytes(t, mainPath)
		testContent := readFileBytes(t, testPath)

		sources, err := adapter.Get([]m.Path{m.Path(mainPath)})
		require.NoError(t, err)
		require.Len(t, sources, 1)

		assertSourceV2(t, &sources[0], mainPath, mainContent, "main", testPath, testContent)
	})

	t.Run("test file input yields no sources", func(t *testing.T) {
		root := t.TempDir()
		testPath := filepath.Join(root, "main_test.go")
		copyExampleFile(t, filepath.Join(examplePath(t, "basic"), "main_test.go"), testPath)

		sources, err := adapter.Get([]m.Path{m.Path(testPath)})
		require.NoError(t, err)
		assert.Len(t, sources, 0)
	})

	t.Run("non-go files are ignored", func(t *testing.T) {
		root := t.TempDir()
		modPath := filepath.Join(root, "go.mod")
		copyExampleFile(t, filepath.Join(examplePath(t, "basic"), "go.mod"), modPath)

		sources, err := adapter.Get([]m.Path{m.Path(root)})
		require.NoError(t, err)
		assert.Len(t, sources, 0)
	})

	t.Run("duplicate roots are de-duplicated", func(t *testing.T) {
		root := t.TempDir()
		mainPath := filepath.Join(root, "main.go")
		copyExampleFile(t, filepath.Join(examplePath(t, "basic"), "main.go"), mainPath)
		mainContent := readFileBytes(t, mainPath)

		sources, err := adapter.Get([]m.Path{m.Path(root), m.Path(root)})
		require.NoError(t, err)
		require.Len(t, sources, 1)

		assertSourceV2(t, &sources[0], mainPath, mainContent, "main", "", nil)
	})

	t.Run("broken source files are skipped", func(t *testing.T) {
		root := t.TempDir()
		brokenPath := filepath.Join(root, "broken.go")
		writeTestFile(t, brokenPath, "package main\nfunc {\n")

		sources, err := adapter.Get([]m.Path{m.Path(root)})
		require.NoError(t, err)
		assert.Len(t, sources, 0)
	})

	t.Run("broken test files are ignored", func(t *testing.T) {
		root := t.TempDir()
		sourcePath := filepath.Join(root, "calc.go")
		testPath := filepath.Join(root, "calc_test.go")
		writeTestFile(t, sourcePath, "package calc\nfunc Sum(a, b int) int { return a + b }\n")
		writeTestFile(t, testPath, "package calc\nfunc {\n")

		sources, err := adapter.Get([]m.Path{m.Path(root)})
		require.NoError(t, err)
		require.Len(t, sources, 1)

		if assert.NotNil(t, sources[0].Origin) {
			assert.Equal(t, m.Path(sourcePath), sources[0].Origin.Path)
		}
		assert.Nil(t, sources[0].Test)
	})
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	writeTestBytes(t, path, []byte(contents))
}

func writeTestBytes(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("failed to create dir %s: %v", path, err)
	}
}

func containsPath(paths []string, target string) bool {
	for _, p := range paths {
		if p == target {
			return true
		}
	}

	return false
}

func findSourceV2ByOrigin(sources []m.Source, origin string) *m.Source {
	for i := range sources {
		if sources[i].Origin == nil {
			continue
		}
		if string(sources[i].Origin.Path) == origin {
			return &sources[i]
		}
	}

	return nil
}

func assertSourceV2(t *testing.T, source *m.Source, originPath string, originContent []byte, pkg string, testPath string, testContent []byte) {
	t.Helper()

	if source == nil {
		require.Fail(t, "source is nil")
	}

	if source.Origin == nil {
		require.Fail(t, "Origin is nil")
	}

	assert.Equal(t, m.Path(originPath), source.Origin.Path)
	assert.Equal(t, hashBytes(originContent), source.Origin.Hash)
	if assert.NotNil(t, source.Package) {
		assert.Equal(t, pkg, *source.Package)
	}

	if testPath == "" {
		assert.Nil(t, source.Test)
		return
	}

	if assert.NotNil(t, source.Test) {
		assert.Equal(t, m.Path(testPath), source.Test.Path)
		assert.Equal(t, hashBytes(testContent), source.Test.Hash)
	}
}

func hashBytes(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}

func examplePath(t *testing.T, elem ...string) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)

	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	parts := append([]string{repoRoot, "examples"}, elem...)

	return filepath.Join(parts...)
}

func copyExampleFile(t *testing.T, src, dst string) {
	t.Helper()
	content := readFileBytes(t, src)
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
	require.NoError(t, os.WriteFile(dst, content, 0o644))
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	return content
}
