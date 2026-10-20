package domain

import (
	"fmt"
	"path/filepath"
	"strings"
)

type coveredRange struct {
	start   int
	end     int
	covered bool
}

// CoverageIndex answers line coverage questions from a parsed Go coverage
// profile. Lines outside every profiled block (such as package-level
// declarations) are unknown rather than uncovered.
type CoverageIndex struct {
	files map[string][]coveredRange
}

// ParseCoverage parses `go test -coverprofile` output. The leading `mode:`
// line and unparseable lines are ignored.
func ParseCoverage(content []byte) (*CoverageIndex, error) {
	idx := &CoverageIndex{files: make(map[string][]coveredRange)}

	for raw := range strings.SplitSeq(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}

		path, r, count, ok := parseProfileLine(line)
		if !ok {
			continue
		}

		r.covered = count > 0
		idx.files[path] = append(idx.files[path], r)
	}

	return idx, nil
}

// parseProfileLine parses one block line of the form
// "path:startLine.col,endLine.col numStmts count".
func parseProfileLine(line string) (path string, r coveredRange, count int, ok bool) {
	colon := strings.LastIndex(line, ":")
	if colon <= 0 {
		return "", coveredRange{}, 0, false
	}

	path = line[:colon]
	rest := line[colon+1:]

	var startLine, startCol, endLine, endCol, numStmts int
	if _, err := fmt.Sscanf(rest, "%d.%d,%d.%d %d %d", &startLine, &startCol, &endLine, &endCol, &numStmts, &count); err != nil {
		return "", coveredRange{}, 0, false
	}

	return path, coveredRange{start: startLine, end: endLine}, count, true
}

// Covers reports whether the given line of shortPath (a project-root-relative
// path) was executed.
func (c *CoverageIndex) Covers(shortPath string, line int) bool {
	covered, _ := c.lookup(shortPath, line)

	return covered
}

// Uncovered reports whether the profile shows the given line of shortPath as
// never executed. Lines the profile knows nothing about return false.
func (c *CoverageIndex) Uncovered(shortPath string, line int) bool {
	covered, known := c.lookup(shortPath, line)

	return known && !covered
}

func (c *CoverageIndex) lookup(shortPath string, line int) (covered, known bool) {
	if c == nil {
		return false, false
	}

	ranges, ok := c.files[c.resolve(shortPath)]
	if !ok {
		return false, false
	}

	for _, r := range ranges {
		if line < r.start || line > r.end {
			continue
		}

		known = true

		if r.covered {
			return true, true
		}
	}

	return false, known
}

// resolve finds the profile path for a project-relative path. Profile paths are
// module-qualified, so the shortest entry ending in shortPath is the match.
func (c *CoverageIndex) resolve(shortPath string) string {
	target := strings.TrimPrefix(filepath.ToSlash(shortPath), "/")

	best := ""

	for path := range c.files {
		if pathMatches(path, target) && (best == "" || len(path) < len(best)) {
			best = path
		}
	}

	return best
}

func pathMatches(profilePath, target string) bool {
	profilePath = filepath.ToSlash(profilePath)
	if profilePath == target {
		return true
	}

	return strings.HasSuffix(profilePath, "/"+target)
}
