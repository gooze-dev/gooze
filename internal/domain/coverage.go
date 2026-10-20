package domain

import (
	"fmt"
	"path/filepath"
	"strings"
)

type coveredRange struct {
	start int
	end   int
}

// CoverageIndex reports whether a given source file line was executed, based on a
// parsed Go coverage profile. Every file in the profile is indexed, but only
// ranges with a positive count are retained, so any line not found is treated
// as uncovered.
type CoverageIndex struct {
	files map[string][]coveredRange
}

// ParseCoverage parses `go test -coverprofile` output, retaining only blocks with
// count > 0. The leading `mode:` line and unparseable lines are ignored.
func ParseCoverage(content []byte) (*CoverageIndex, error) {
	idx := &CoverageIndex{files: make(map[string][]coveredRange)}

	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}

		path, r, count, ok := parseProfileLine(line)
		if !ok {
			continue
		}

		// Record every file the profile mentions, even when no block has a
		// positive count, so Covers can resolve an ambiguous short path to the
		// right file instead of falling through to a longer, unrelated match.
		if count > 0 {
			idx.files[path] = append(idx.files[path], r)
		} else if _, seen := idx.files[path]; !seen {
			idx.files[path] = nil
		}
	}

	return idx, nil
}

// parseProfileLine parses one block line of the form
// "path:startLine.col,endLine.col numStmts count".
func parseProfileLine(line string) (path string, r coveredRange, count int, ok bool) {
	colon := strings.LastIndex(line, ":")
	if colon < 0 {
		return "", coveredRange{}, 0, false
	}

	path = line[:colon]
	if path == "" {
		return "", coveredRange{}, 0, false
	}

	rest := line[colon+1:]

	var startLine, startCol, endLine, endCol, numStmts int
	if _, err := fmt.Sscanf(rest, "%d.%d,%d.%d %d %d", &startLine, &startCol, &endLine, &endCol, &numStmts, &count); err != nil {
		return "", coveredRange{}, 0, false
	}

	return path, coveredRange{start: startLine, end: endLine}, count, true
}

// Covers reports whether the given line of shortPath (a project-root-relative
// path) was executed. A profile entry matches when its path ends with shortPath
// at a separator boundary. When several entries match (for example a root
// main.go and a nested cmd/x/main.go both match "main.go"), the shortest
// profile path is the one that corresponds to shortPath, because the correct
// entry is the module path plus shortPath and every other match carries extra
// directories.
func (c *CoverageIndex) Covers(shortPath string, line int) bool {
	if c == nil {
		return false
	}

	target := strings.TrimPrefix(filepath.ToSlash(shortPath), "/")

	var (
		bestPath string
		found    bool
	)

	for path := range c.files {
		if !pathMatches(path, target) {
			continue
		}

		if !found || len(path) < len(bestPath) {
			bestPath, found = path, true
		}
	}

	if !found {
		return false
	}

	for _, r := range c.files[bestPath] {
		if line >= r.start && line <= r.end {
			return true
		}
	}

	return false
}

func pathMatches(profilePath, target string) bool {
	profilePath = filepath.ToSlash(profilePath)
	if profilePath == target {
		return true
	}

	return strings.HasSuffix(profilePath, "/"+target)
}
