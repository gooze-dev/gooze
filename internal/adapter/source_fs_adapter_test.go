package adapter

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
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
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		for _, forbidden := range []string{nestedDir, filepath.Join(nestedDir, "child.go")} {
			if containsPath(visited, forbidden) {
				t.Fatalf("Walk() unexpectedly visited %s when recursive is false", forbidden)
			}
		}

		if !containsPath(visited, filepath.Join(root, "main.go")) {
			t.Fatalf("Walk() did not visit top-level file")
		}
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
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		if !containsPath(visited, child) {
			t.Fatalf("Walk() did not visit nested file when recursive")
		}
	})
}

func TestLocalSourceFSAdapter_ReadFile(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	content := "package main\n" + "func main() {}\n"
	writeTestFile(t, path, content)

	got, err := adapter.ReadFile(m.Path(path))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(got) != content {
		t.Fatalf("ReadFile() = %q, want %q", string(got), content)
	}
}

func TestLocalSourceFSAdapter_HashFile(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	content := []byte("package main\nfunc main() {}\n")
	writeTestBytes(t, path, content)

	expected := fmt.Sprintf("%x", sha256.Sum256(content))

	hash, err := adapter.HashFile(m.Path(path))
	if err != nil {
		t.Fatalf("HashFile() error = %v", err)
	}

	if hash != expected {
		t.Fatalf("HashFile() = %s, want %s", hash, expected)
	}
}

func TestLocalSourceFSAdapter_DetectTestFile(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	root := t.TempDir()
	source := filepath.Join(root, "calc.go")
	testFile := filepath.Join(root, "calc_test.go")
	writeTestFile(t, source, "package calc\n")
	writeTestFile(t, testFile, "package calc\n")

	got, err := adapter.DetectTestFile(m.Path(source))
	if err != nil {
		t.Fatalf("DetectTestFile() error = %v", err)
	}

	if got != m.Path(testFile) {
		t.Fatalf("DetectTestFile() = %s, want %s", got, testFile)
	}

	t.Run("returns empty path when test file missing", func(t *testing.T) {
		missingSrc := filepath.Join(root, "other.go")
		writeTestFile(t, missingSrc, "package main\n")

		got, err := adapter.DetectTestFile(m.Path(missingSrc))
		if err != nil {
			t.Fatalf("DetectTestFile() error = %v", err)
		}

		if got != "" {
			t.Fatalf("DetectTestFile() = %s, want empty path", got)
		}
	})
}

func TestLocalSourceFSAdapter_FileInfo(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	writeTestFile(t, path, "package main\n")

	info, err := adapter.FileInfo(m.Path(path))
	if err != nil {
		t.Fatalf("FileInfo() error = %v", err)
	}

	if info.IsDir() {
		t.Fatalf("FileInfo() reported file as directory")
	}

	dirInfo, err := adapter.FileInfo(m.Path(root))
	if err != nil {
		t.Fatalf("FileInfo() error = %v", err)
	}

	if !dirInfo.IsDir() {
		t.Fatalf("FileInfo() reported directory as file")
	}
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
		t.Fatalf("failed to create nested dir: %v", err)
	}

	got, err := adapter.FindProjectRoot(m.Path(filepath.Join(subDir, "file.go")))
	if err != nil {
		t.Fatalf("FindProjectRoot() error = %v", err)
	}

	if got != m.Path(goModDir) {
		t.Fatalf("FindProjectRoot() = %s, want %s", got, goModDir)
	}
}

func TestLocalSourceFSAdapter_CreateTempDirAndRemoveAll(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	tmp, err := adapter.CreateTempDir("gooze-test-*")
	if err != nil {
		t.Fatalf("CreateTempDir() error = %v", err)
	}

	if fi, err := os.Stat(string(tmp)); err != nil || !fi.IsDir() {
		t.Fatalf("CreateTempDir() did not create directory, stat err=%v, isDir=%v", err, err == nil && fi.IsDir())
	}

	filePath := filepath.Join(string(tmp), "file.go")
	writeTestFile(t, filePath, "package main\n")

	if err := adapter.RemoveAll(tmp); err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}

	if _, err := os.Stat(string(tmp)); !os.IsNotExist(err) {
		t.Fatalf("RemoveAll() did not remove directory, stat err=%v", err)
	}
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
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := adapter.CopyDir(m.Path(src), m.Path(dst)); err != nil {
		t.Fatalf("CopyDir() error = %v", err)
	}

	// Check that files exist in destination
	if _, err := os.Stat(filepath.Join(dst, "sub", "main.go")); err != nil {
		t.Fatalf("CopyDir() did not copy nested file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "extra.go")); err != nil {
		t.Fatalf("CopyDir() did not copy top-level file: %v", err)
	}
}

func TestLocalSourceFSAdapter_PathHelpers(t *testing.T) {
	adapter := NewLocalSourceFSAdapter()

	base := m.Path("/tmp/project")
	target := m.Path("/tmp/project/sub/dir/file.go")

	rel, err := adapter.RelPath(base, target)
	if err != nil {
		t.Fatalf("RelPath() error = %v", err)
	}

	if string(rel) != filepath.Join("sub", "dir", "file.go") {
		t.Fatalf("RelPath() = %s, want %s", rel, filepath.Join("sub", "dir", "file.go"))
	}

	joined := adapter.JoinPath("/tmp", "project", "sub", "file.go")
	if string(joined) != filepath.Join("/tmp", "project", "sub", "file.go") {
		t.Fatalf("JoinPath() = %s, want %s", joined, filepath.Join("/tmp", "project", "sub", "file.go"))
	}
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
