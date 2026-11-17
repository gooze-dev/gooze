package domain

import "fmt"

// FileLines is the lines with survivors a view lists for one file, and how
// many more of its lines the per-file cap leaves out.
type FileLines struct {
	Path   string
	Lines  []SurvivorLine
	Hidden int
}

// ListedLines groups survivors, sorted by path and line, per file and line,
// and keeps the first limit lines of each file; a limit of 0 keeps them all.
func ListedLines(survivors []Survivor, limit int) []FileLines {
	var (
		files []FileLines
		file  *FileLines
	)

	for _, line := range groupByLine(survivors) {
		switch {
		// Equivalent: the lines are sorted by path, so a path that differs is greater.
		case file == nil || file.Path != line.Path: //gooze:ignore comparison(<)
			files = append(files, FileLines{Path: line.Path, Lines: []SurvivorLine{line}})
			file = &files[len(files)-1]
		case len(file.Lines) == limit:
			file.Hidden++
		default:
			file.Lines = append(file.Lines, line)
		}
	}

	return files
}

// hiddenAnnotation is the notice that stands for the lines of file the cap
// of limit leaves without an annotation.
func hiddenAnnotation(file FileLines, limit int, prefix string) string {
	noun := "lines"
	// Equivalent: the notice is for at least one line, so <= 1 is == 1.
	if file.Hidden == 1 { //gooze:ignore comparison
		noun = "line"
	}

	title := fmt.Sprintf("%d more %s with mutants", file.Hidden, noun)

	return "::notice file=" + escapeAnnotationProperty(prefix+file.Path) + ",title=" + escapeAnnotationProperty(title) +
		"::" + fmt.Sprintf("report.max_per_file lists %d per file", limit)
}
