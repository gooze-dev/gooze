package adapter

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// UnifiedDiffer renders unified diffs between original and mutated sources.
type UnifiedDiffer struct{}

// NewUnifiedDiffer constructs a UnifiedDiffer.
func NewUnifiedDiffer() *UnifiedDiffer {
	return &UnifiedDiffer{}
}

// Diff returns a unified diff of original against mutated, or nil when both are empty.
func (d *UnifiedDiffer) Diff(original, mutated []byte) []byte {
	if len(original) == 0 && len(mutated) == 0 {
		return nil
	}

	ud := difflib.UnifiedDiff{
		FromFile: "original",
		ToFile:   "mutated",
		Context:  3,
		A:        difflib.SplitLines(string(ensureTrailingNewline(original))),
		B:        difflib.SplitLines(string(ensureTrailingNewline(mutated))),
	}

	text, err := difflib.GetUnifiedDiffString(ud)
	if err != nil {
		return nil
	}

	return []byte(text)
}

func ensureTrailingNewline(content []byte) []byte {
	if len(content) == 0 || strings.HasSuffix(string(content), "\n") {
		return content
	}

	return append(content, '\n')
}
