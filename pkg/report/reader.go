package report

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Reader reads a reports directory. DirReader reads it from the local file
// system; MockReader fakes it in tests.
type Reader interface {
	// ReadDir returns every report in dir, in file name order.
	ReadDir(dir string) ([]Report, error)
	// WalkDir calls fn for every report file in dir, in file name order, and
	// stops at the first error.
	WalkDir(dir string, fn func(path string, r Report) error) error
	// ReadIndex reads IndexFileName from dir.
	ReadIndex(dir string) (Index, error)
}

// DirReader is the Reader for reports on the local file system.
type DirReader struct{}

// NewDirReader returns a Reader for reports on the local file system.
func NewDirReader() DirReader {
	return DirReader{}
}

// ReadDir returns every report in dir, in file name order.
func (DirReader) ReadDir(dir string) ([]Report, error) {
	var reports []Report

	err := WalkDir(dir, func(_ string, r Report) error {
		reports = append(reports, r)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return reports, nil
}

// WalkDir calls fn for every report file in dir, in file name order. It skips
// subdirectories, IndexFileName and files without a .yaml or .yml extension,
// and stops at the first error.
func (DirReader) WalkDir(dir string, fn func(path string, r Report) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read reports directory: %w", err)
	}

	for _, entry := range entries {
		if !isReportFile(entry) {
			continue
		}

		path := filepath.Join(dir, entry.Name())

		r, err := ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return err
		}

		if err := fn(path, r); err != nil {
			return err
		}
	}

	return nil
}

// ReadIndex reads IndexFileName from dir.
func (DirReader) ReadIndex(dir string) (Index, error) {
	path := filepath.Join(dir, IndexFileName)

	data, err := os.ReadFile(path) // #nosec G304 -- the index lives in a caller-chosen reports directory
	if err != nil {
		return Index{}, fmt.Errorf("read index file %s: %w", path, err)
	}

	var index Index
	if err := yaml.Unmarshal(data, &index); err != nil {
		return Index{}, fmt.Errorf("unmarshal index file %s: %w", path, err)
	}

	return index, nil
}

// ReadDir returns every report in dir using DirReader.
func ReadDir(dir string) ([]Report, error) {
	return DirReader{}.ReadDir(dir)
}

// WalkDir walks the reports in dir using DirReader.
func WalkDir(dir string, fn func(path string, r Report) error) error {
	return DirReader{}.WalkDir(dir, fn)
}

// ReadIndex reads IndexFileName from dir using DirReader.
func ReadIndex(dir string) (Index, error) {
	return DirReader{}.ReadIndex(dir)
}

// ReadFile reads one report file.
func ReadFile(path string) (Report, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- reading a caller-chosen report file is the purpose
	if err != nil {
		return Report{}, fmt.Errorf("read report file %s: %w", path, err)
	}

	var r Report
	if err := yaml.Unmarshal(data, &r); err != nil {
		return Report{}, fmt.Errorf("unmarshal report file %s: %w", path, err)
	}

	return r, nil
}

func isReportFile(entry os.DirEntry) bool {
	if entry.IsDir() {
		return false
	}

	name := entry.Name()
	if name == IndexFileName {
		return false
	}

	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}
