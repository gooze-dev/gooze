package adapter

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

// Issue #140: which sources a changed test puts in diff scope must not depend
// on how test file names sort.
func TestLocalSourceFSAdapter_UnpairedSourcesStayOutOfDiffScopeOfTestChanges(t *testing.T) {
	root := t.TempDir()
	cmd := filepath.Join(root, "cmd")
	require.NoError(t, os.MkdirAll(cmd, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module p\n"), 0o644))

	for _, name := range []string{"file.go", "root.go", "contributor.go", "webhook.go", "file_test.go", "root_test.go", "branch_delete_test.go"} {
		require.NoError(t, os.WriteFile(filepath.Join(cmd, name), []byte("package cmd\n"), 0o644))
	}

	sources, err := NewLocalSourceFSAdapter().Get(context.Background(), []m.Path{m.Path(cmd)})
	require.NoError(t, err)

	inScope := func(changed string) []string {
		scope := domain.NewDiffScope(domain.ChangeSet{Files: map[string]domain.FileChange{
			filepath.Join(cmd, changed): {Whole: true},
		}})

		var names []string
		for _, source := range sources {
			if scope.InScope(source) {
				names = append(names, filepath.Base(source.Origin.FullPath))
			}
		}

		return names
	}

	assert.Equal(t, []string{"file.go"}, inScope("file_test.go"))
	assert.Equal(t, []string{"root.go"}, inScope("root_test.go"))
	assert.Empty(t, inScope("branch_delete_test.go"))
}
