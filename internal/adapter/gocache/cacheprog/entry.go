// Package cacheprog is the GOCACHEPROG server that keeps the builds of mutants
// out of the user's Go build cache: reads go to gooze's own cache first, then
// to the shared GOCACHE, and writes go only to gooze's.
package cacheprog

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// entry is a cache entry: the output stored under an action ID.
type entry struct {
	outputID []byte
	size     int64
	time     time.Time
	path     string // the output's file
}

// cacheFile returns the path of an action ("a") or output ("d") file in a
// cache laid out like Go's disk cache.
func cacheFile(dir string, id []byte, kind string) string {
	name := hex.EncodeToString(id)
	return filepath.Join(dir, name[:2], name+"-"+kind)
}

// readEntry reads the entry of actionID from a cache laid out like Go's disk
// cache, without touching it. The action file holds
// "v1 <action> <output> <size> <time>". A missing or invalid file is a miss.
func readEntry(dir string, actionID []byte) (entry, bool) {
	data, err := os.ReadFile(cacheFile(dir, actionID, "a"))
	if err != nil {
		return entry{}, false
	}

	e, ok := parseIndex(string(data), actionID)
	if !ok {
		return entry{}, false
	}

	e.path = cacheFile(dir, e.outputID, "d")

	info, err := os.Stat(e.path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != e.size {
		return entry{}, false
	}

	return e, true
}

// parseIndex parses the action file of actionID, all but the output's path.
func parseIndex(index string, actionID []byte) (entry, bool) {
	fields := strings.Fields(index)
	if len(fields) != 5 || fields[0] != "v1" || fields[1] != hex.EncodeToString(actionID) {
		return entry{}, false
	}

	outputID, err := hex.DecodeString(fields[2])
	if err != nil || len(outputID) != len(actionID) {
		return entry{}, false
	}

	// Equivalent: strconv treats bitSize 0 as 64.
	size, sizeErr := strconv.ParseInt(fields[3], 10, 64) //gooze:ignore numbers
	// Equivalent: strconv treats bitSize 0 as 64.
	nanos, timeErr := strconv.ParseInt(fields[4], 10, 64) //gooze:ignore numbers

	return entry{outputID: outputID, size: size, time: time.Unix(0, nanos)}, sizeErr == nil && timeErr == nil
}
