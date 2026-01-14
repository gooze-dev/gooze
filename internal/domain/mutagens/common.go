package mutagens

import (
	m "github.com/mouse-blink/gooze/internal/model"
)

// FindScopeType determines which scope a line belongs to.
func FindScopeType(scopes []m.CodeScope, line int) m.ScopeType {
	for _, scope := range scopes {
		if line >= scope.StartLine && line <= scope.EndLine {
			return scope.Type
		}
	}

	// Default to function scope if not found in any scope
	return m.ScopeFunction
}
