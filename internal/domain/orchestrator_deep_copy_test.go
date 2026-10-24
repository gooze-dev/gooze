package domain_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var logs bytes.Buffer

	defaultLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(defaultLogger) })

	return &logs
}

// copyFS fakes the filesystem of deep-copy runs: a mutation of root/main.go is
// copied to /tmp/copy-N, and restoring the original fails with restoreErr.
type copyFS struct {
	copied  []m.Path
	removed []m.Path
}

func newCopyFS(t *testing.T, restoreErr, removeErr error) (*copyFS, domain.Workspace) {
	t.Helper()

	fake := &copyFS{}
	fsAdapter := domainmocks.NewMockSourceFSAdapter(t)
	trAdapter := domainmocks.NewMockTestRunnerAdapter(t)

	fsAdapter.EXPECT().FindProjectRoot(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, path m.Path) (m.Path, error) {
		return m.Path(filepath.Dir(string(path))), nil
	})
	fsAdapter.EXPECT().CreateTempDir(mock.Anything, "gooze-mutation-*").RunAndReturn(func(context.Context, string) (m.Path, error) {
		return m.Path(fmt.Sprintf("/tmp/copy-%d", len(fake.copied)+1)), nil
	})
	fsAdapter.EXPECT().CopyDir(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, root, _ m.Path) error {
		fake.copied = append(fake.copied, root)
		return nil
	})
	fsAdapter.EXPECT().RelPath(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _, path m.Path) (m.Path, error) {
		return m.Path(filepath.Base(string(path))), nil
	})
	fsAdapter.EXPECT().JoinPath(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, elem ...string) m.Path {
		return m.Path(filepath.Join(elem...))
	})
	fsAdapter.EXPECT().ReadFile(mock.Anything, mock.Anything).Return([]byte("original"), nil)
	fsAdapter.EXPECT().WriteFile(mock.Anything, mock.Anything, mock.Anything, os.FileMode(0o600)).
		RunAndReturn(func(_ context.Context, _ m.Path, content []byte, _ os.FileMode) error {
			if string(content) == "original" {
				return restoreErr
			}

			return nil
		})
	fsAdapter.EXPECT().RemoveAll(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, path m.Path) error {
		fake.removed = append(fake.removed, path)
		return removeErr
	}).Maybe()
	trAdapter.EXPECT().RunGoTest(mock.Anything, mock.Anything, mock.Anything, "").Return("ok", nil)

	return fake, domain.NewOrchestrator(fsAdapter, trAdapter).NewWorkspace(domain.WorkspaceDeepCopy)
}

func mutationIn(root string) m.Mutation {
	return m.Mutation{
		ID:          "id-" + root,
		Type:        mgen.Arithmetic,
		MutatedCode: []byte("mutated"),
		Source: rpt.Source{
			Origin: &rpt.File{FullPath: root + "/main.go"},
			Test:   &rpt.File{FullPath: root + "/main_test.go"},
		},
	}
}

func TestWorkspace_DeepCopy_ReusesTheCopyOfTheSameProject(t *testing.T) {
	fake, ws := newCopyFS(t, nil, nil)
	ctx := context.Background()

	for _, root := range []string{"/a", "/a", "/b", "/a"} {
		_, err := ws.Run(ctx, mutationIn(root))
		require.NoError(t, err)
	}

	ws.Close(ctx)

	assert.Equal(t, []m.Path{"/a", "/b", "/a"}, fake.copied, "a project is copied again only when the root changes")
	assert.Equal(t, []m.Path{"/tmp/copy-1", "/tmp/copy-2", "/tmp/copy-3"}, fake.removed)
}

func TestWorkspace_DeepCopy_FailedRestoreDiscardsTheCopy(t *testing.T) {
	logs := captureLogs(t)
	fake, ws := newCopyFS(t, errors.New("disk full"), nil)
	ctx := context.Background()

	for range 2 {
		_, err := ws.Run(ctx, mutationIn("/a"))
		require.NoError(t, err)
	}

	ws.Close(ctx)

	assert.Equal(t, []m.Path{"/a", "/a"}, fake.copied, "a copy holding a mutant is never reused")
	assert.Equal(t, []m.Path{"/tmp/copy-1", "/tmp/copy-2"}, fake.removed)
	assert.Contains(t, logs.String(), "discarding workspace copy")
}

func TestWorkspace_DeepCopy_CloseLogsOnlyAFailedRemove(t *testing.T) {
	logs := captureLogs(t)
	_, ws := newCopyFS(t, nil, nil)

	_, err := ws.Run(context.Background(), mutationIn("/a"))
	require.NoError(t, err)
	ws.Close(context.Background())
	assert.NotContains(t, logs.String(), "Failed to cleanup temp dir")

	_, ws = newCopyFS(t, nil, errors.New("busy"))

	_, err = ws.Run(context.Background(), mutationIn("/a"))
	require.NoError(t, err)
	ws.Close(context.Background())
	assert.Contains(t, logs.String(), "Failed to cleanup temp dir")
}

func TestWorkspace_DeepCopy_FailedCopyIsRemoved(t *testing.T) {
	for _, removeErr := range []error{nil, errors.New("busy")} {
		logs := captureLogs(t)
		fsAdapter := domainmocks.NewMockSourceFSAdapter(t)
		ctx := context.Background()

		fsAdapter.EXPECT().FindProjectRoot(ctx, m.Path("/a/main.go")).Return(m.Path("/a"), nil)
		fsAdapter.EXPECT().CreateTempDir(ctx, "gooze-mutation-*").Return(m.Path("/tmp/copy-1"), nil)
		fsAdapter.EXPECT().CopyDir(ctx, m.Path("/a"), m.Path("/tmp/copy-1")).Return(errors.New("no space"))
		fsAdapter.EXPECT().RemoveAll(mock.Anything, m.Path("/tmp/copy-1")).Return(removeErr).Once()

		ws := domain.NewOrchestrator(fsAdapter, domainmocks.NewMockTestRunnerAdapter(t)).NewWorkspace(domain.WorkspaceDeepCopy)

		_, err := ws.Run(ctx, mutationIn("/a"))
		require.ErrorContains(t, err, "failed to copy project: no space")
		ws.Close(ctx)

		if removeErr == nil {
			assert.NotContains(t, logs.String(), "Failed to clean up temp dir after copy failure")
		} else {
			assert.Contains(t, logs.String(), "Failed to clean up temp dir after copy failure")
		}
	}
}
