package domain

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkg "gooze.dev/pkg/gooze/pkg"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

type fakeBuildCache struct {
	prepared bool
	closeErr error
}

func (c *fakeBuildCache) Prepare(context.Context, GoCache, []string) error {
	c.prepared = true
	return nil
}

func (c *fakeBuildCache) Close(context.Context) error { return c.closeErr }

func noSources(t *testing.T) pkg.FileSpill[rpt.Source] {
	t.Helper()

	sources, err := pkg.NewFileSpill[rpt.Source]()
	require.NoError(t, err)
	t.Cleanup(func() { closeSpill("sources", sources) })

	return sources
}

func TestPrepareBuildCache_OnlyIsolatedModePrepares(t *testing.T) {
	for mode, want := range map[GoCacheMode]bool{"": false, GoCacheShared: false, GoCacheIsolated: true} {
		cache := &fakeBuildCache{}
		w := &workflow{buildCache: cache}

		closeCache, err := w.prepareBuildCache(context.Background(), GoCache{Mode: mode}, noSources(t))
		require.NoError(t, err)
		closeCache()

		assert.Equal(t, want, cache.prepared, string(mode))
	}
}

func TestPrepareBuildCache_WarnsOnlyWhenCloseFails(t *testing.T) {
	var logs bytes.Buffer

	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(original) })

	for closeErr, want := range map[error]bool{nil: false, errors.New("full"): true} {
		logs.Reset()

		w := &workflow{buildCache: &fakeBuildCache{closeErr: closeErr}}
		closeCache, err := w.prepareBuildCache(context.Background(), GoCache{Mode: GoCacheIsolated}, noSources(t))
		require.NoError(t, err)
		closeCache()

		assert.Equal(t, want, bytes.Contains(logs.Bytes(), []byte("Failed to trim")))
	}
}
