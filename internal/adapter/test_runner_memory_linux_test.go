package adapter

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupRSS(t *testing.T) {
	proc := t.TempDir()
	stat := func(pid int, comm string, pgrp, rssPages int) {
		t.Helper()

		dir := filepath.Join(proc, strconv.Itoa(pid))
		require.NoError(t, os.Mkdir(dir, 0o700))

		// The fields of proc(5)'s /proc/pid/stat: pgrp is the 5th, rss the 24th.
		line := strconv.Itoa(pid) + " (" + comm + ") S 1 " + strconv.Itoa(pgrp) +
			" 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 " + strconv.Itoa(rssPages) + " 25 26\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte(line), 0o600))
	}

	stat(100, "go", 100, 10)
	// A command name may hold spaces and parentheses.
	stat(101, "a b) (c", 100, 5)
	stat(102, "x.test", 100, 0)
	// Another group, whose pid and other fields equal the group asked for.
	stat(200, "other", 201, 1000)
	stat(201, "100", 201, 100)

	// What else /proc holds: entries that aren't processes, a process that
	// ended between the listing and the read, and lines that can't be read.
	require.NoError(t, os.Mkdir(filepath.Join(proc, "self"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "self", "stat"), []byte("100 (go) S 1 100 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 999 25\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "uptime"), []byte("1 (x) S 1 100 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 999 25\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(proc, "300"), 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(proc, "301"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "301", "stat"), []byte("301 (short) S 1 100 6\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(proc, "302"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "302", "stat"), []byte("no command name\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(proc, "303"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "303", "stat"), []byte("303 (bad) S 1 100 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 many 25\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(proc, "304"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "304", "stat"), []byte("304 (bad) S 1 group 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 7 25\n"), 0o600))

	page := int64(os.Getpagesize())

	require.Equal(t, 15*page, groupRSS(proc, 100))
	require.Equal(t, 1100*page, groupRSS(proc, 201))
	require.Zero(t, groupRSS(proc, 999), "no process of the group")
	require.Zero(t, groupRSS(filepath.Join(proc, "missing"), 100), "no proc directory")
}
