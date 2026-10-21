package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var releaseTestNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func newTestReleaseChecker(t *testing.T, handler http.HandlerFunc) (*ModuleProxyReleaseChecker, *int) {
	t.Helper()

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	return &ModuleProxyReleaseChecker{
		URL:       srv.URL,
		CachePath: filepath.Join(t.TempDir(), "gooze", releaseCacheFileName),
		TTL:       time.Hour,
		Client:    srv.Client(),
		Now:       func() time.Time { return releaseTestNow },
	}, &hits
}

func writeReleaseCache(t *testing.T, c *ModuleProxyReleaseChecker, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(c.CachePath), 0o750))
	require.NoError(t, os.WriteFile(c.CachePath, []byte(content), 0o600))
}

func serveVersion(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Version":"` + version + `","Time":"2026-10-20T17:12:32Z"}`))
	}
}

func TestModuleProxyReleaseChecker_Cached(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cache     string // "" means no cache file
		wantTag   string
		wantFresh bool
	}{
		"missing":        {"", "", false},
		"corrupt":        {"{not json", "", false},
		"fresh":          {`{"tag":"v1.2.5","checked_at":"2026-10-21T11:30:00Z"}`, "v1.2.5", true},
		"stale":          {`{"tag":"v1.0.0","checked_at":"2026-10-21T10:59:59Z"}`, "v1.0.0", false},
		"cached failure": {`{"tag":"","checked_at":"2026-10-21T11:30:00Z"}`, "", true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			checker, hits := newTestReleaseChecker(t, serveVersion("v1.3.0"))
			if tt.cache != "" {
				writeReleaseCache(t, checker, tt.cache)
			}

			tag, fresh := checker.Cached()
			assert.Equal(t, tt.wantTag, tag)
			assert.Equal(t, tt.wantFresh, fresh)
			assert.Equal(t, 0, *hits, "Cached must never touch the network")
		})
	}
}

func TestModuleProxyReleaseChecker_CachedWithoutCachePath(t *testing.T) {
	t.Parallel()

	tag, fresh := (&ModuleProxyReleaseChecker{Now: time.Now}).Cached()
	assert.Empty(t, tag)
	assert.False(t, fresh)
}

func TestModuleProxyReleaseChecker_RefreshWritesCache(t *testing.T) {
	t.Parallel()

	checker, hits := newTestReleaseChecker(t, serveVersion("v1.3.0"))
	writeReleaseCache(t, checker, `{"tag":"v1.0.0","checked_at":"2026-10-21T10:00:00Z"}`)

	tag, err := checker.Refresh(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "v1.3.0", tag)
	assert.Equal(t, 1, *hits)

	tag, fresh := checker.Cached()
	assert.Equal(t, "v1.3.0", tag)
	assert.True(t, fresh)

	entries, err := os.ReadDir(filepath.Dir(checker.CachePath))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the atomic write must not leave temp files behind")
}

func TestModuleProxyReleaseChecker_RefreshCachesFailures(t *testing.T) {
	t.Parallel()

	checker, _ := newTestReleaseChecker(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := checker.Refresh(context.Background())
	require.Error(t, err)

	tag, fresh := checker.Cached()
	assert.Empty(t, tag)
	assert.True(t, fresh, "a failed lookup should not be retried within the TTL")
}

func TestModuleProxyReleaseChecker_RefreshOffline(t *testing.T) {
	t.Parallel()

	checker, _ := newTestReleaseChecker(t, serveVersion("v1.3.0"))
	checker.URL = "http://127.0.0.1:1" // connection refused
	checker.Client = &http.Client{Timeout: time.Second}

	_, err := checker.Refresh(context.Background())
	require.Error(t, err)

	_, fresh := checker.Cached()
	assert.True(t, fresh, "an offline lookup is cached so the next run does not retry")
}

func TestModuleProxyReleaseChecker_RefreshBadJSON(t *testing.T) {
	t.Parallel()

	checker, _ := newTestReleaseChecker(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	})
	checker.CachePath = ""

	_, err := checker.Refresh(context.Background())
	require.Error(t, err)
}

func TestModuleProxyReleaseChecker_RefreshCanceledIsNotCached(t *testing.T) {
	t.Parallel()

	checker, _ := newTestReleaseChecker(t, serveVersion("v1.3.0"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := checker.Refresh(ctx)
	require.ErrorIs(t, err, context.Canceled)

	_, statErr := os.Stat(checker.CachePath)
	assert.True(t, os.IsNotExist(statErr), "a canceled lookup must not be cached")
}
