package mutagens

import (
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
)

func TestFindScopeType(t *testing.T) {
	scopes := []m.CodeScope{
		{StartLine: 1, EndLine: 5, Type: m.ScopeGlobal},
		{StartLine: 6, EndLine: 15, Type: m.ScopeFunction},
		{StartLine: 16, EndLine: 20, Type: m.ScopeInit},
	}

	tests := []struct {
		line     int
		expected m.ScopeType
	}{
		{1, m.ScopeGlobal},
		{3, m.ScopeGlobal},
		{5, m.ScopeGlobal},
		{6, m.ScopeFunction},
		{10, m.ScopeFunction},
		{15, m.ScopeFunction},
		{16, m.ScopeInit},
		{18, m.ScopeInit},
		{20, m.ScopeInit},
		{21, m.ScopeFunction}, // Outside any scope, defaults to function
		{0, m.ScopeFunction},  // Before any scope, defaults to function
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := FindScopeType(scopes, tt.line)
			if result != tt.expected {
				t.Errorf("FindScopeType(scopes, %d) = %v, expected %v", tt.line, result, tt.expected)
			}
		})
	}
}

func TestFindScopeType_EmptyScopes(t *testing.T) {
	var scopes []m.CodeScope

	result := FindScopeType(scopes, 10)
	if result != m.ScopeFunction {
		t.Errorf("FindScopeType(empty, 10) = %v, expected %v", result, m.ScopeFunction)
	}
}

func TestFindScopeType_NoMatchingScope(t *testing.T) {
	scopes := []m.CodeScope{
		{StartLine: 1, EndLine: 5, Type: m.ScopeGlobal},
	}

	// Test line before first scope
	result := FindScopeType(scopes, 0)
	if result != m.ScopeFunction {
		t.Errorf("FindScopeType(scopes, 0) = %v, expected %v", result, m.ScopeFunction)
	}

	// Test line after last scope
	result = FindScopeType(scopes, 10)
	if result != m.ScopeFunction {
		t.Errorf("FindScopeType(scopes, 10) = %v, expected %v", result, m.ScopeFunction)
	}
}
