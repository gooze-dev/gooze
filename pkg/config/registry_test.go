package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRegistry_DelegatesToPackageFunctions(t *testing.T) {
	var registry Registry = NewRegistry()

	assert.Equal(t, Schema(), registry.Schema())
	assert.Equal(t, Keys(), registry.Keys())
	assert.Equal(t, IsConfigFile("gooze.yaml"), registry.IsConfigFile("gooze.yaml"))

	field, ok := registry.Lookup(KeyRunParallel)
	want, wantOK := Lookup(KeyRunParallel)
	assert.Equal(t, wantOK, ok)
	assert.Equal(t, want, field)
}

func TestMockRegistry_FakesRegistry(t *testing.T) {
	mock := NewMockRegistry(t)
	mock.EXPECT().Lookup("run.parallel").Return(Field{Name: "parallel", Kind: KindInt}, true).Once()

	var registry Registry = mock

	field, ok := registry.Lookup("run.parallel")
	require.True(t, ok)
	assert.Equal(t, KindInt, field.Kind)
}
