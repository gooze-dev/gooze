package report

import (
	"strconv"
	"strings"
)

// Change is one original source line a mutation replaced or removed, as
// recorded in a report's Diff.
type Change struct {
	// Line is the one-based line number in the original source.
	Line int
	// Original is the original line, without the diff's "-" prefix.
	Original string
	// Mutated is the line that replaced it, without the diff's "+" prefix, or
	// "" when the mutation removed the line.
	Mutated string
}

// DiffChanges extracts the original lines a report Diff (a unified diff with
// "--- original", "+++ mutated" and "@@ -a,b +c,d @@" hunks) removes, pairing
// each with the added line that replaced it, in diff order. gooze's hunks carry
// three lines of context, so the changed line is not the hunk's first line. It
// returns nil for an empty or malformed diff.
func DiffChanges(diff []byte) []Change {
	var (
		changes []Change
		old     = -1 // next original line (one-based); -1 until the first hunk
		// pending indexes the first unpaired removal in changes. Equivalent
		// mutants: it is reset at every hunk header before it is read.
		pending = 0 //gooze:ignore numbers
	)

	for line := range strings.SplitSeq(string(diff), "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			old = hunkOldStart(line)
			pending = len(changes)
		// Equivalent mutants: old is -1 before the first hunk and at least 1 after.
		case old < 0 || strings.HasPrefix(line, "\\"): //gooze:ignore comparison numbers
			// File headers before the first hunk and "\ No newline at end of file".
		case strings.HasPrefix(line, "-"):
			changes = append(changes, Change{Line: old, Original: line[1:]})
			old++
		case strings.HasPrefix(line, "+"):
			// Equivalent mutants: pending never exceeds len(changes).
			if pending < len(changes) { //gooze:ignore comparison
				changes[pending].Mutated = line[1:]
				pending++
			}
		default: // context line (" text", or "" for a blank line)
			old++
			pending = len(changes)
		}
	}

	return changes
}

// hunkOldStart returns the one-based original start line from a hunk header
// ("@@ -a,b +c,d @@" or "@@ -a +c @@"), or -1 when the header is malformed.
func hunkOldStart(header string) int {
	_, after, ok := strings.Cut(header, "-")
	if !ok {
		return -1
	}

	num, _, _ := strings.Cut(after, " ")
	num, _, _ = strings.Cut(num, ",")

	n, err := strconv.Atoi(num)
	if err != nil || n < 1 {
		return -1
	}

	return n
}
