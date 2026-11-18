package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestGuardIndex_At(t *testing.T) {
	index := guardIndex{{start: 5, end: 9, guards: m.Position{Line: 2, Column: 3}}}

	assert.Nil(t, index.at(4))
	assert.Equal(t, &m.Position{Line: 2, Column: 3}, index.at(5))
	assert.Equal(t, &m.Position{Line: 2, Column: 3}, index.at(8))
	assert.Nil(t, index.at(9), "the end of a condition is past it")
}
