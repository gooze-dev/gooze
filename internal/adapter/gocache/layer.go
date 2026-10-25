package gocache

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// inUse is how long a trim spares a file after its last use: a concurrent
// run may still read it.
const inUse = time.Hour

// layer is gooze's own cache, laid out like Go's disk cache.
type layer struct {
	dir string
}

// layerFile is a file of the layer, for trimming.
type layerFile struct {
	path    string
	size    int64
	modTime time.Time
}

// get reads the entry of actionID and marks its files used.
func (l layer) get(actionID []byte) (entry, bool) {
	e, ok := readEntry(l.dir, actionID)
	if ok {
		now := time.Now()
		_ = os.Chtimes(cacheFile(l.dir, actionID, "a"), now, now)
		_ = os.Chtimes(e.path, now, now)
	}

	return e, ok
}

// put stores body as the output of actionID and returns the output's path.
func (l layer) put(actionID, outputID, body []byte, now time.Time) (string, error) {
	path := cacheFile(l.dir, outputID, "d")
	if err := writeFile(path, body); err != nil {
		return "", err
	}

	index := fmt.Sprintf("v1 %x %x %20d %20d\n", actionID, outputID, len(body), now.UnixNano())

	return path, writeFile(cacheFile(l.dir, actionID, "a"), []byte(index))
}

// writeFile writes data to a temporary file and renames it to path, so a
// concurrent reader never sees a partial file.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}

	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}

	if err == nil {
		err = os.Rename(f.Name(), path)
	}

	if err != nil {
		_ = os.Remove(f.Name())
	}

	return err
}

// trim deletes the least recently used files until the layer holds at most
// limit bytes, sparing the ones used within inUse of now.
func (l layer) trim(limit int64, now time.Time) error {
	files, total, err := l.files()
	if err != nil {
		return err
	}

	for _, f := range files {
		if total <= limit || now.Sub(f.modTime) < inUse {
			return nil
		}

		if err := os.Remove(f.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}

		total -= f.size
	}

	return nil
}

// files returns the layer's files, least recently used first, and their
// total size. A missing layer has none.
func (l layer) files() ([]layerFile, int64, error) {
	var (
		files []layerFile
		total int64
	)

	err := filepath.WalkDir(l.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		files = append(files, layerFile{path: path, size: info.Size(), modTime: info.ModTime()})
		total += info.Size()

		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}

	slices.SortFunc(files, func(a, b layerFile) int { return a.modTime.Compare(b.modTime) })

	return files, total, err
}
