package domain

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// plainMutagen streams without options and records the types it got.
type plainMutagen struct {
	Mutagen
	types []mgen.Type
}

func (p *plainMutagen) StreamMutations(_ context.Context, _ rpt.Source, fn func(m.Mutation) error, types ...mgen.Type) error {
	p.types = types
	return fn(m.Mutation{ID: "plain"})
}

// optionsMutagen streams with options and records them.
type optionsMutagen struct {
	plainMutagen
	opts MutationOptions
}

func (o *optionsMutagen) StreamMutationsWith(_ context.Context, _ rpt.Source, opts MutationOptions, fn func(m.Mutation) error, types ...mgen.Type) error {
	o.opts, o.types = opts, types
	return fn(m.Mutation{ID: "with options"})
}

func TestWorkflow_StreamMutations(t *testing.T) {
	errStop := errors.New("stop")
	opts := MutationOptions{EnumBounds: true}

	var got []string

	collect := func(mutation m.Mutation) error {
		got = append(got, mutation.ID)
		return errStop
	}

	withOptions := &optionsMutagen{}
	err := (&workflow{mutagen: withOptions}).streamMutations(context.Background(), rpt.Source{}, opts, collect, mgen.Comparison)
	require.ErrorIs(t, err, errStop)
	require.Equal(t, opts, withOptions.opts)
	require.Equal(t, []mgen.Type{mgen.Comparison}, withOptions.types)

	plain := &plainMutagen{}
	err = (&workflow{mutagen: plain}).streamMutations(context.Background(), rpt.Source{}, opts, collect, mgen.Comparison)
	require.ErrorIs(t, err, errStop)
	require.Equal(t, []mgen.Type{mgen.Comparison}, plain.types)

	require.Equal(t, []string{"with options", "plain"}, got)
}
