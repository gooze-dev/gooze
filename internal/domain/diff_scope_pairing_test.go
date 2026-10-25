package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gooze.dev/pkg/gooze/internal/domain"
)

// A source without its own x_test.go is run with another test file of its
// package, the first in sort order. Changing that file doesn't put the source
// in scope, any more than changing any other test file of the package would.
func TestDiffScope_OnlyThePairedTestPutsASourceInScope(t *testing.T) {
	t.Parallel()

	contributor := diffSource("/p/cmd/contributor.go", "/p/cmd/file_test.go")
	webhook := diffSource("/p/cmd/webhook.go", "/p/cmd/file_test.go")
	file := diffSource("/p/cmd/file.go", "/p/cmd/file_test.go")
	root := diffSource("/p/cmd/root.go", "/p/cmd/root_test.go")

	scopeOf := func(changed ...string) *domain.DiffScope {
		files := map[string]domain.FileChange{}
		for _, path := range changed {
			files[path] = domain.FileChange{Whole: true}
		}

		return domain.NewDiffScope(domain.ChangeSet{Files: files})
	}

	for _, changed := range []string{"/p/cmd/file_test.go", "/p/cmd/root_test.go", "/p/cmd/branch_delete_test.go"} {
		scope := scopeOf(changed)
		assert.False(t, scope.InScope(contributor), changed)
		assert.False(t, scope.InScope(webhook), changed)
		assert.False(t, scope.Includes(webhook, 1), changed)
		assert.NotNil(t, scope.ReportScope(webhook), changed)
	}

	assert.True(t, scopeOf("/p/cmd/file_test.go").InScope(file))
	assert.True(t, scopeOf("/p/cmd/file_test.go").Includes(file, 1))
	assert.Nil(t, scopeOf("/p/cmd/file_test.go").ReportScope(file))
	assert.True(t, scopeOf("/p/cmd/root_test.go").InScope(root))
	assert.False(t, scopeOf("/p/cmd/root_test.go").InScope(file))

	assert.True(t, scopeOf("/p/cmd/webhook.go").InScope(webhook), "its own changes still count")
	assert.False(t, scopeOf("/p/cmd/webhook.go").InScope(contributor))
}
