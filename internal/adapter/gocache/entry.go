// Package gocache keeps the builds of mutants out of the user's Go build
// cache. The go command reaches it through GOCACHEPROG: reads go to gooze's
// own cache first, then to the shared GOCACHE, and writes go only to gooze's.
package gocache

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

	fields := strings.Fields(string(data))
	if len(fields) != 5 || fields[0] != "v1" || fields[1] != hex.EncodeToString(actionID) {
		return entry{}, false
	}

	outputID, err := hex.DecodeString(fields[2])
	if err != nil || len(outputID) != len(actionID) {
		return entry{}, false
	}

	size, sizeErr := strconv.ParseInt(fields[3], 10, 64)
	nanos, timeErr := strconv.ParseInt(fields[4], 10, 64)
	path := cacheFile(dir, outputID, "d")

	info, err := os.Stat(path)
	if sizeErr != nil || timeErr != nil || err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return entry{}, false
	}

	return entry{outputID: outputID, size: size, time: time.Unix(0, nanos), path: path}, true
}
