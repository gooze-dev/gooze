// Package mutagens provides functions to generate code mutations.
package mutagens

import "go/token"

func offsetForPos(fset *token.FileSet, pos token.Pos) (int, bool) {
	file := fset.File(pos)
	if file == nil {
		return 0, false
	}

	return file.Offset(pos), true
}

func replaceRange(content []byte, start, end int, replacement string) []byte {
	if start < 0 || end < start || end > len(content) {
		return content
	}

	mutated := make([]byte, 0, len(content)-(end-start)+len(replacement))
	mutated = append(mutated, content[:start]...)
	mutated = append(mutated, []byte(replacement)...)
	mutated = append(mutated, content[end:]...)

	return mutated
}
