package cmd

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	"gooze.dev/pkg/gooze/pkg/config"
)

// stubReleases is a ReleaseChecker whose Refresh blocks until release is
// closed (or forever), standing in for a slow or missing network.
type stubReleases struct {
	cachedTag string
	fresh     bool
	latest    string
	release   chan struct{}
	refreshed chan struct{}
}

func newStubReleases(cachedTag string, fresh bool, latest string) *stubReleases {
	return &stubReleases{cachedTag: cachedTag, fresh: fresh, latest: latest, release: make(chan struct{}), refreshed: make(chan struct{}, 1)}
}

func (s *stubReleases) Cached() (string, bool) { return s.cachedTag, s.fresh }

func (s *stubReleases) Refresh(ctx context.Context) (string, error) {
	s.refreshed <- struct{}{}
	select {
	case <-s.release:
		return s.latest, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

type recordingNotifier struct {
	mu      sync.Mutex
	notices []domain.UpdateNotice
	got     chan struct{}
}

func newRecordingNotifier() *recordingNotifier {
	return &recordingNotifier{got: make(chan struct{}, 4)}
}

func (r *recordingNotifier) DisplayUpdateNotice(_ context.Context, notice domain.UpdateNotice) {
	r.mu.Lock()
	r.notices = append(r.notices, notice)
	r.mu.Unlock()
	r.got <- struct{}{}
}

func (r *recordingNotifier) all() []domain.UpdateNotice {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]domain.UpdateNotice(nil), r.notices...)
}

func TestCheckForUpdate_NeverWaitsOnTheNetwork(t *testing.T) {
	// A stale cache triggers a refresh that never completes (dead network).
	checker := newStubReleases("", false, "v1.3.0")

	start := time.Now()
	notice := checkForUpdate(checker, "v1.2.0", nil)

	assert.Nil(t, notice)
	assert.Less(t, time.Since(start), 100*time.Millisecond)

	select {
	case <-checker.refreshed:
	case <-time.After(2 * time.Second):
		t.Fatal("stale cache should start a background refresh")
	}
}

func TestCheckForUpdate_CachedNotice(t *testing.T) {
	checker := newStubReleases("v1.3.0", true, "")
	notifier := newRecordingNotifier()

	notice := checkForUpdate(checker, "v1.2.0", notifier)

	require.NotNil(t, notice)
	assert.Equal(t, domain.UpdateNotice{Current: "v1.2.0", Latest: "v1.3.0"}, *notice)
	assert.Equal(t, []domain.UpdateNotice{*notice}, notifier.all())

	select {
	case <-checker.refreshed:
		t.Fatal("a fresh cache must not refresh")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCheckForUpdate_RefreshUpdatesRunningUI(t *testing.T) {
	checker := newStubReleases("v1.2.0", false, "v1.4.0")
	notifier := newRecordingNotifier()

	assert.Nil(t, checkForUpdate(checker, "v1.2.0", notifier), "only the cached value is printed at exit")

	close(checker.release)

	select {
	case <-notifier.got:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh result should reach the running UI")
	}

	assert.Equal(t, []domain.UpdateNotice{{Current: "v1.2.0", Latest: "v1.4.0"}}, notifier.all())
}

func TestCheckForUpdate_DevelBuildSkipsEverything(t *testing.T) {
	checker := newStubReleases("v1.3.0", false, "v1.3.0")

	assert.Nil(t, checkForUpdate(checker, "(devel)", nil))

	select {
	case <-checker.refreshed:
		t.Fatal("devel builds must not refresh")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestUpdateCheckEnabled(t *testing.T) {
	root := &cobra.Command{Use: "gooze"}
	run := &cobra.Command{Use: "run"}
	completion := &cobra.Command{Use: "completion"}
	bash := &cobra.Command{Use: "bash"}
	complete := &cobra.Command{Use: cobra.ShellCompRequestCmd}

	root.AddCommand(run, completion, complete)
	completion.AddCommand(bash)

	assert.True(t, updateCheckEnabled(run))
	assert.False(t, updateCheckEnabled(bash))
	assert.False(t, updateCheckEnabled(complete))

	viper.Set(config.KeyNoUpdateCheck, true)
	t.Cleanup(func() { viper.Set(config.KeyNoUpdateCheck, false) })
	assert.False(t, updateCheckEnabled(run))
}

func TestPrintUpdateNotice(t *testing.T) {
	original := pendingNotice
	t.Cleanup(func() { pendingNotice = original })

	notice := domain.UpdateNotice{Current: "v1.2.0", Latest: "v1.3.0"}

	var buf bytes.Buffer

	pendingNotice = nil
	printUpdateNotice(context.Background(), &buf)
	assert.Empty(t, buf.String())

	pendingNotice = &notice
	printUpdateNotice(context.Background(), &buf)
	assert.Equal(t, notice.String()+"\n", buf.String())

	buf.Reset()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	printUpdateNotice(ctx, &buf)
	assert.Empty(t, buf.String(), "an interrupted command prints nothing")
}
