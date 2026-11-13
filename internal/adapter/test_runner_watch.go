package adapter

import (
	"io/fs"
	"path/filepath"
	"time"
)

// Equivalent: a shorter interval measures more often and kills the same tests.
const limitCheckInterval = 100 * time.Millisecond //gooze:ignore arithmetic numbers

func watchLimit(kill func(), over func() bool) func() bool {
	done := make(chan struct{})
	killed := make(chan bool)

	go func() {
		ticker := time.NewTicker(limitCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				killed <- false
				return
			case <-ticker.C:
				if over() {
					kill()

					killed <- true

					return
				}
			}
		}
	}()

	return func() bool {
		close(done)
		return <-killed
	}
}

func watchDisk(kill func(), dirs []string, limit int64) func() bool {
	if limit <= 0 {
		return func() bool { return false }
	}

	return watchLimit(kill, func() bool { return dirsSize(dirs) > limit })
}

func dirsSize(dirs []string) int64 {
	var size int64

	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil //nolint:nilerr // a file removed since the listing counts for nothing.
			}

			if info, err := entry.Info(); err == nil {
				size += info.Size()
			}

			return nil
		})
	}

	return size
}
