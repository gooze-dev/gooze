package domain

import (
	"context"
	"go/ast"
	"go/token"
	"os"

	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// SourceFSAdapter abstracts filesystem-specific operations that the domain layer
// relies on when scanning user projects. It intentionally hides direct `os`
// access so the workflow logic can be tested without touching the disk.
//
//nolint:interfacebloat // A richer interface keeps workflow logic decoupled from os/fs.
type SourceFSAdapter interface {
	Get(ctx context.Context, roots []m.Path, ignore ...string) ([]rpt.Source, error)

	// Stream scans the provided roots and emits each discovered source on the
	// returned channel as it is found, instead of buffering them all. The source
	// channel is closed when the scan finishes; the error channel then yields a
	// single error (or nothing on success). Callers should cancel ctx if they
	// stop reading early so the scanning goroutine can exit.
	Stream(ctx context.Context, roots []m.Path, ignore ...string) (<-chan rpt.Source, <-chan error)

	// Walk traverses the provided root path. When recursive is false the
	// implementation should limit itself to the root directory (no sub-dirs).
	Walk(ctx context.Context, root m.Path, recursive bool, fn FilepathWalkFunc) error

	// ReadFile loads a file from disk and returns its contents.
	ReadFile(ctx context.Context, path m.Path) ([]byte, error)

	// HashFile returns a stable fingerprint (e.g. SHA-256) for the file at path.
	HashFile(ctx context.Context, path m.Path) (string, error)

	// DetectTestFile attempts to find a Go test file that matches the provided
	// source file. This allows the domain to auto-link source/test pairs.
	DetectTestFile(ctx context.Context, sourcePath m.Path) (m.Path, error)

	// FileInfo returns metadata for a path so the domain can check existence or
	// distinguish between files and directories when necessary.
	FileInfo(ctx context.Context, path m.Path) (os.FileInfo, error)

	// FindProjectRoot searches for go.mod file walking up the directory tree.
	FindProjectRoot(ctx context.Context, startPath m.Path) (m.Path, error)

	// CreateTempDir creates a temporary directory for mutation testing.
	CreateTempDir(ctx context.Context, pattern string) (m.Path, error)

	// RemoveAll removes a directory and all its contents.
	RemoveAll(ctx context.Context, path m.Path) error

	// CopyDir recursively copies a directory tree.
	CopyDir(ctx context.Context, src, dst m.Path) error

	// WriteFile writes content to a file with the given permissions.
	WriteFile(ctx context.Context, path m.Path, content []byte, perm os.FileMode) error
	// RelPath returns the relative path from base to target.
	RelPath(ctx context.Context, base, target m.Path) (m.Path, error)

	// JoinPath joins path elements into a single path.
	JoinPath(ctx context.Context, elem ...string) m.Path
}

// FilepathWalkFunc mirrors the callback shape used by filepath.WalkDir. It is
// defined here to avoid leaking the standard-library type directly into the
// domain layer.
type FilepathWalkFunc func(path string, info os.FileInfo, err error) error

// GoFileAdapter encapsulates Go-specific parsing and scope-detection logic so
// the domain layer can focus on mutation rules while delegating compilation
// details to an infrastructure component.
type GoFileAdapter interface {
	// Parse builds an AST using the provided file set and optional source bytes.
	Parse(ctx context.Context, fileSet *token.FileSet, filename string, src []byte) (*ast.File, error)
}

// TestRunnerAdapter abstracts test execution operations for mutation testing.
type TestRunnerAdapter interface {
	// RunGoTest runs 'go test' for the package containing testFile, inside
	// workDir. Returns the combined stdout/stderr output and any error.
	RunGoTest(ctx context.Context, workDir, testFile string) (output string, err error)
}

// ReportStore persists and loads mutation reports.
type ReportStore interface {
	SaveReports(ctx context.Context, path m.Path, reports pkg.FileSpill[m.Report]) error
	RegenerateIndex(ctx context.Context, path m.Path) error
	LoadSpillReports(ctx context.Context, path m.Path) (pkg.FileSpill[m.Report], error)
	CheckUpdates(ctx context.Context, path m.Path, sources pkg.FileSpill[rpt.Source]) (changed, deleted pkg.FileSpill[rpt.Source], err error)
	CleanReports(ctx context.Context, path m.Path, sources pkg.FileSpill[rpt.Source]) error
}

// RegistryOptions tweaks how the registry is contacted.
type RegistryOptions struct {
	PlainHTTP bool // use http instead of https
	Insecure  bool // skip TLS certificate verification
}

// OCIRegistry stores and retrieves a reports directory as an OCI artifact.
type OCIRegistry interface {
	Push(ctx context.Context, ref string, dir m.Path, opts RegistryOptions) error
	Pull(ctx context.Context, ref string, dir m.Path, opts RegistryOptions) error
}

// Differ renders a human-readable diff between the original and mutated source.
type Differ interface {
	Diff(original, mutated []byte) []byte
}
