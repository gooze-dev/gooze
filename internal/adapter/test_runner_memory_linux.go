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

// procDir lists the processes of the machine, as proc(5) describes.
const procDir = "/proc"

// memoryCheckInterval is how often a go test's memory is measured. Memory
// taken between two measurements is over the limit until the next one.
//
// Equivalent: a shorter interval measures more often and kills the same tests.
const memoryCheckInterval = 100 * time.Millisecond //gooze:ignore arithmetic numbers

// watchMemory kills the process group of cmd, which leads it, as soon as its
// processes together hold more than limit bytes; 0 is no limit. The returned
// function ends the watch and reports whether it killed the group.
func watchMemory(cmd *exec.Cmd, limit int64) func() bool {
	return watchGroupMemory(procDir, cmd.Process.Pid, limit)
}

// watchGroupMemory is watchMemory for the process group pgid, measured in
// procDir.
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
					// The group may have ended by itself.
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

// groupRSS returns the resident memory, in bytes, of the processes of the
// process group pgid that procDir (/proc) lists.
func groupRSS(procDir string, pgid int) int64 {
	dir, err := os.Open(procDir) //nolint:gosec // /proc, or a test's copy of it.
	if err != nil {
		return 0
	}

	defer dir.Close() //nolint:errcheck // read only.

	// The entries read before an error are the ones to count.
	names, _ := dir.Readdirnames(0)

	var pages int64

	for _, name := range names {
		// A process is named by its pid; self and thread-self name gooze again.
		if _, err := strconv.Atoi(name); err != nil {
			continue
		}

		// A process that ended since the listing has no stat to read.
		stat, err := os.ReadFile(filepath.Join(procDir, name, "stat")) //nolint:gosec // as procDir.
		if err != nil {
			continue
		}

		pages += statRSS(stat, pgid)
	}

	return pages * int64(os.Getpagesize())
}

// statRSS returns the resident pages a /proc/pid/stat line gives, when its
// process belongs to the process group pgid, and 0 otherwise.
func statRSS(stat []byte, pgid int) int64 {
	// The fields of proc(5) after the command name, which is in parentheses
	// and may hold spaces and parentheses itself: the state is the first.
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
