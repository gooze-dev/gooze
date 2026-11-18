package domain

import (
	"cmp"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	m "gooze.dev/pkg/gooze/internal/model"
)

type coveredRange struct {
	start   int
	end     int
	covered bool
	// startColumn is the first column of the block on its start line, and
	// endColumn the one after its last on its end line.
	startColumn int
	endColumn   int
}

// contains reports whether the block holds the given column of line.
func (r coveredRange) contains(line, column int) bool {
	at := m.Position{Line: line, Column: column}

	return !before(at, m.Position{Line: r.start, Column: r.startColumn}) &&
		before(at, m.Position{Line: r.end, Column: r.endColumn})
}

// before reports whether a comes before b in a file.
func before(a, b m.Position) bool {
	return cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column)) < 0
}

// CoverageIndex answers line coverage questions from a parsed Go coverage
// profile. Lines outside every profiled block (such as package-level
// declarations) are unknown rather than uncovered.
type CoverageIndex struct {
	files map[string][]coveredRange
	// Conditions also holds a mutation uncovered when the code its condition
	// guards never ran, see NotCovered.
	Conditions bool
}

// ParseCoverage parses `go test -coverprofile` output. Lines that aren't a
// block, such as the leading `mode:` line and empty ones, are ignored.
func ParseCoverage(content []byte) (*CoverageIndex, error) {
	idx := &CoverageIndex{files: make(map[string][]coveredRange)}

	for raw := range strings.SplitSeq(string(content), "\n") {
		path, r, count, ok := parseProfileLine(strings.TrimSpace(raw))
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

	return path, coveredRange{start: startLine, end: endLine, startColumn: startCol, endColumn: endCol}, count, true
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

// UncoveredAt reports whether the profile shows the given column of a line of
// shortPath as never executed. Unlike Uncovered it tells the blocks of one
// line apart, such as a condition and the block it guards.
func (c *CoverageIndex) UncoveredAt(shortPath string, line, column int) bool {
	covered, known := c.find(shortPath, func(r coveredRange) bool { return r.contains(line, column) })

	return known && !covered
}

// NotCovered reports whether no test runs what the mutation changes: its
// line never ran or, with Conditions, the code its condition guards didn't.
func (c *CoverageIndex) NotCovered(mutation m.Mutation) bool {
	path := mutation.Source.Origin.ShortPath
	if c.Uncovered(path, mutation.Line) {
		return true
	}

	if c == nil || !c.Conditions || mutation.Guards == nil {
		return false
	}

	return c.UncoveredAt(path, mutation.Guards.Line, mutation.Guards.Column)
}

func (c *CoverageIndex) lookup(shortPath string, line int) (covered, known bool) {
	return c.find(shortPath, func(r coveredRange) bool { return line >= r.start && line <= r.end })
}

// find reports whether shortPath has a block that holds, and whether one of
// those was executed.
func (c *CoverageIndex) find(shortPath string, holds func(coveredRange) bool) (covered, known bool) {
	if c == nil {
		return false, false
	}

	for _, r := range c.files[c.resolve(shortPath)] {
		if !holds(r) {
			continue
		}

		known = true
		covered = covered || r.covered
	}

	return covered, known
}

// resolve finds the profile path for a project-relative path. Profile paths are
// module-qualified, so the shortest entry ending in shortPath is the match,
// and of two that short the first by name; "" without a match.
func (c *CoverageIndex) resolve(shortPath string) string {
	target := strings.TrimPrefix(filepath.ToSlash(shortPath), "/")

	matches := slices.DeleteFunc(slices.Collect(maps.Keys(c.files)), func(path string) bool {
		return !pathMatches(path, target)
	})

	slices.SortFunc(matches, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(a), len(b)), cmp.Compare(a, b))
	})

	return cmp.Or(matches...)
}

func pathMatches(profilePath, target string) bool {
	profilePath = filepath.ToSlash(profilePath)
	if profilePath == target {
		return true
	}

	return strings.HasSuffix(profilePath, "/"+target)
}
