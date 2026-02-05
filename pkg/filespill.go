// Package pkg is a package that provides utilities for gooze.
package pkg

import (
	"encoding/gob"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// FileSpill is a generic interface for spilling items of type T to disk.
type FileSpill[T any] interface {
	Len() uint64
	Path() string
	Append(item T) error
	AppendBatch(items []T) error
	Get(index uint64) (T, error)
	Range(f func(index uint64, item T) error) error
	Close() error
}

type fileSpillImpl[T any] struct {
	path    string
	file    *os.File
	encoder *gob.Encoder
	mu      sync.Mutex
	length  uint64
}

// Append implements FileSpill.
func (f *fileSpillImpl[T]) Append(item T) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.encoder.Encode(item); err != nil {
		slog.Error("failed to encode item", "path", f.path, "index", f.length, "error", err)
		return fmt.Errorf("failed to encode item: %w", err)
	}

	f.length++
	slog.Debug("appended item", "path", f.path, "index", f.length-1)

	return nil
}

// Path implements FileSpill.
func (f *fileSpillImpl[T]) Path() string {
	return f.path
}

// AppendBatch implements FileSpill.
func (f *fileSpillImpl[T]) AppendBatch(items []T) error {
	for _, item := range items {
		if err := f.Append(item); err != nil {
			return err
		}
	}

	return nil
}

// Close implements FileSpill.
func (f *fileSpillImpl[T]) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.file != nil {
		if err := f.file.Close(); err != nil {
			slog.Error("failed to close file", "path", f.path, "error", err)
			return err
		}

		slog.Debug("closed filespill", "path", f.path, "length", f.length)
	}

	return nil
}

// Get implements FileSpill.
func (f *fileSpillImpl[T]) Get(index uint64) (T, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if index >= f.length {
		var zero T

		slog.Warn("get index out of bounds", "path", f.path, "index", index, "length", f.length)

		return zero, fmt.Errorf("index %d out of bounds (length %d)", index, f.length)
	}

	file, err := os.Open(f.path)
	if err != nil {
		var zero T

		slog.Error("failed to open file for get", "path", f.path, "error", err)

		return zero, fmt.Errorf("failed to open file: %w", err)
	}

	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("failed to close file", "path", f.path, "error", err)
		}
	}()

	decoder := gob.NewDecoder(file)

	var item T

	for i := uint64(0); i <= index; i++ {
		if err := decoder.Decode(&item); err != nil {
			var zero T

			slog.Error("failed to decode item", "path", f.path, "index", i, "error", err)

			return zero, fmt.Errorf("failed to decode item at index %d: %w", i, err)
		}
	}

	slog.Debug("got item", "path", f.path, "index", index)

	return item, nil
}

// Len implements FileSpill.
func (f *fileSpillImpl[T]) Len() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.length
}

// Range implements FileSpill.
func (f *fileSpillImpl[T]) Range(fn func(index uint64, item T) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	file, err := os.Open(f.path)
	if err != nil {
		slog.Error("failed to open file for range", "path", f.path, "error", err)
		return fmt.Errorf("failed to open file: %w", err)
	}

	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("failed to close file", "path", f.path, "error", err)
		}
	}()

	decoder := gob.NewDecoder(file)

	var item T

	for i := range f.length {
		if err := decoder.Decode(&item); err != nil {
			slog.Error("failed to decode item during range", "path", f.path, "index", i, "error", err)
			return fmt.Errorf("failed to decode item at index %d: %w", i, err)
		}

		if err := fn(i, item); err != nil {
			slog.Warn("range callback error", "path", f.path, "index", i, "error", err)
			return err
		}
	}

	slog.Debug("range completed", "path", f.path, "count", f.length)

	return nil
}

// NewFileSpill creates a new FileSpill for items of type T.
func NewFileSpill[T any]() (FileSpill[T], error) {
	tmpDir := "/tmp/filespill"
	if err := os.MkdirAll(tmpDir, 0o750); err != nil {
		slog.Error("failed to create temp directory", "path", tmpDir, "error", err)
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	file, err := os.CreateTemp(tmpDir, "spill-*.gob")
	if err != nil {
		slog.Error("failed to create temp file", "path", tmpDir, "error", err)
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	slog.Debug("created filespill", "path", file.Name())

	return &fileSpillImpl[T]{
		path:    file.Name(),
		file:    file,
		encoder: gob.NewEncoder(file),
		length:  0,
	}, nil
}
