package adapter

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const procDir = "/proc"

// Equivalent: a shorter interval measures more often and kills the same tests.
const memoryCheckInterval = 100 * time.Millisecond //gooze:ignore arithmetic numbers

func watchMemory(cmd *exec.Cmd, limit int64) func() bool {
	return watchGroupMemory(procDir, cmd.Process.Pid, limit)
}

func watchGroupMemory(procDir string, pgid int, limit int64) func() bool {
	if limit <= 0 {
		return func() bool { return false }
	}

	done := make(chan struct{})
	killed := make(chan bool)

	go func() {
		ticker := time.NewTicker(memoryCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				killed <- false
				return
			case <-ticker.C:
				if groupRSS(procDir, pgid) > limit {
					_ = syscall.Kill(-pgid, syscall.SIGKILL)

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

func groupRSS(procDir string, pgid int) int64 {
	dir, err := os.Open(procDir) //nolint:gosec // /proc, or a test's copy of it.
	if err != nil {
		return 0
	}

	defer dir.Close() //nolint:errcheck // read only.

	names, _ := dir.Readdirnames(0)

	var pages int64

	for _, name := range names {
		if _, err := strconv.Atoi(name); err != nil {
			continue
		}

		stat, err := os.ReadFile(filepath.Join(procDir, name, "stat")) //nolint:gosec // as procDir.
		if err != nil {
			continue
		}

		pages += statRSS(stat, pgid)
	}

	return pages * int64(os.Getpagesize())
}

func statRSS(stat []byte, pgid int) int64 {
	const pgrpField, rssField = 2, 21

	fields := bytes.Fields(stat[bytes.LastIndexByte(stat, ')')+1:])
	if len(fields) <= rssField {
		return 0
	}

	if pgrp, err := strconv.Atoi(string(fields[pgrpField])); err != nil || pgrp != pgid {
		return 0
	}

	rss, err := strconv.Atoi(string(fields[rssField]))
	if err != nil {
		return 0
	}

	return int64(rss)
}
