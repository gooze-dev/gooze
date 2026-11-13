package adapter

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

const procDir = "/proc"

const pgrpField, sessionField, rssField = 2, 3, 21

func watchMemory(cmd *exec.Cmd, limit int64) func() bool {
	return watchGroupMemory(procDir, cmd.Process.Pid, limit)
}

func watchGroupMemory(procDir string, pgid int, limit int64) func() bool {
	if limit <= 0 {
		return func() bool { return false }
	}

	kill := func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) }

	return watchLimit(kill, func() bool { return groupRSS(procDir, pgid) > limit })
}

func groupRSS(procDir string, pgid int) int64 {
	var pages int64

	eachProcess(procDir, func(_ int, fields [][]byte) {
		pages += statRSS(fields, pgid)
	})

	return pages * int64(os.Getpagesize())
}

func eachProcess(procDir string, visit func(pid int, fields [][]byte)) {
	dir, err := os.Open(procDir) //nolint:gosec // /proc, or a test's copy of it.
	if err != nil {
		return
	}

	defer dir.Close() //nolint:errcheck // read only.

	names, _ := dir.Readdirnames(0)

	for _, name := range names {
		pid, err := strconv.Atoi(name)
		if err != nil {
			continue
		}

		stat, err := os.ReadFile(filepath.Join(procDir, name, "stat")) //nolint:gosec // as procDir.
		if err != nil {
			continue
		}

		fields := bytes.Fields(stat[bytes.LastIndexByte(stat, ')')+1:])
		if len(fields) > rssField {
			visit(pid, fields)
		}
	}
}

func statRSS(fields [][]byte, pgid int) int64 {
	if pgrp, err := strconv.Atoi(string(fields[pgrpField])); err != nil || pgrp != pgid {
		return 0
	}

	rss, err := strconv.Atoi(string(fields[rssField]))
	if err != nil {
		return 0
	}

	return int64(rss)
}
