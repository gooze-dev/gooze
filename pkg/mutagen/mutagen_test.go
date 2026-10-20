package mutagen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypes(t *testing.T) {
	assert.Equal(t, Type{Name: "arithmetic", Version: 1}, Arithmetic)
	assert.Equal(t, Type{Name: "boolean", Version: 1}, Boolean)
	assert.Equal(t, Type{Name: "numbers", Version: 1}, Numbers)
	assert.Equal(t, Type{Name: "comparison", Version: 1}, Comparison)
	assert.Equal(t, Type{Name: "logical", Version: 1}, Logical)
	assert.Equal(t, Type{Name: "unary", Version: 1}, Unary)
	assert.Equal(t, Type{Name: "branch", Version: 1}, Branch)
	assert.Equal(t, Type{Name: "statement", Version: 1}, Statement)
	assert.Equal(t, Type{Name: "loop", Version: 1}, Loop)
}

func TestType_String(t *testing.T) {
	assert.Equal(t, "loop", Loop.String())
}

func TestNames(t *testing.T) {
	assert.Equal(t, []string{
		"arithmetic", "boolean", "numbers", "comparison", "logical",
		"unary", "branch", "statement", "loop",
	}, Names())
}

func TestAll(t *testing.T) {
	all := All()
	require.Len(t, all, 9)

	types := make([]Type, 0, len(all))
	for _, mg := range all {
		types = append(types, mg.Type)
		assert.NotEmpty(t, mg.Doc, mg.Name)
	}

	assert.Equal(t, []Type{Arithmetic, Boolean, Numbers, Comparison, Logical, Unary, Branch, Statement, Loop}, types)
}

func TestAll_ReturnsCopy(t *testing.T) {
	all := All()
	all[0].Name = "changed"

	assert.Equal(t, "arithmetic", All()[0].Name)
}

func TestLookup(t *testing.T) {
	mg, ok := Lookup("Comparison")
	require.True(t, ok)
	assert.Equal(t, Comparison, mg.Type)
	assert.Equal(t, "Mutates comparison operators (<, >, <=, >=, ==, !=).", mg.Doc)

	mg, ok = Lookup(" LOOP ")
	require.True(t, ok)
	assert.Equal(t, Loop, mg.Type)

	_, ok = Lookup("arith")
	assert.False(t, ok)

	_, ok = Lookup("")
	assert.False(t, ok)
}

func TestIsValid(t *testing.T) {
	assert.True(t, IsValid("arithmetic"))
	assert.True(t, IsValid("Unary"))
	assert.False(t, IsValid("ignorearithmetic"))
	assert.False(t, IsValid("arithmetic,"))
}

func TestVersions(t *testing.T) {
	assert.Equal(t, map[string]int{
		"arithmetic": 1, "boolean": 1, "numbers": 1, "comparison": 1, "logical": 1,
		"unary": 1, "branch": 1, "statement": 1, "loop": 1,
	}, Versions())
}
