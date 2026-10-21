package adapter

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
)

func TestParseUnifiedDiff(t *testing.T) {
	t.Parallel()

	diff := `diff --git a/calc.go b/calc.go
index 1111111..2222222 100644
--- a/calc.go
+++ b/calc.go
@@ -3 +3 @@ func InRange(n, lo, hi int) bool {
-	if n < lo {
+	if n <= lo {
@@ -10,0 +11,2 @@ func Add(a, b int) int {
+	x := a
+	y := b
@@ -20,2 +21,0 @@ func Sub(a, b int) int {
-	gone := 1
-	gone2 := 2
diff --git a/new.go b/new.go
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package calc
+
+func New() {}
diff --git a/old.go b/old.go
deleted file mode 100644
index 4444444..0000000
--- a/old.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package calc
-func Old() {}
diff --git a/same.go b/moved/same.go
similarity index 100%
rename from same.go
rename to moved/same.go
diff --git a/edit.go b/moved/edit.go
similarity index 90%
rename from edit.go
rename to moved/edit.go
index 5555555..6666666 100644
--- a/edit.go
+++ b/moved/edit.go
@@ -2 +2 @@
-func Edit() {}
+func Edit() { _ = 1 }
diff --git a/mode.go b/mode.go
old mode 100644
new mode 100755
diff --git "a/sp\"ace.go" "b/sp\"ace.go"
index 7777777..8888888 100644
--- "a/sp\"ace.go"
+++ "b/sp\"ace.go"
@@ -1 +1 @@
-package a
+package b
`

	assert.Equal(t, map[string]domain.FileChange{
		"calc.go":       {Lines: []domain.LineRange{{Start: 3, End: 3}, {Start: 11, End: 12}}},
		"new.go":        {Whole: true},
		"moved/edit.go": {Lines: []domain.LineRange{{Start: 2, End: 2}}},
		`sp"ace.go`:     {Lines: []domain.LineRange{{Start: 1, End: 1}}},
	}, parseUnifiedDiff(diff))
}

func TestParseUnifiedDiff_Empty(t *testing.T) {
	t.Parallel()

	assert.Empty(t, parseUnifiedDiff(""))
}

func TestParseHunkNewRange(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		start, count int
		ok           bool
	}{
		"@@ -3 +3 @@":            {3, 1, true},
		"@@ -3,2 +5,4 @@ func x": {5, 4, true},
		"@@ -20,2 +21,0 @@":      {21, 0, true},
		"@@ -0,0 +1 @@":          {1, 1, true},
		"@@ malformed":           {0, 0, false},
		"@@ -1 +x,2 @@":          {0, 0, false},
		"@@ -1 +2,y @@":          {0, 0, false},
	}

	for header, want := range tests {
		start, count, ok := parseHunkNewRange(header)
		assert.Equal(t, want.ok, ok, header)
		assert.Equal(t, want.start, start, header)
		assert.Equal(t, want.count, count, header)
	}
}

func TestRunGit_ErrorNamesSubcommandAndGitMessage(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	// git prints a reason on stderr: the error carries its first line.
	_, err = runGit(context.Background(), git, dir, "rev-parse", "--show-prefix")
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "git rev-parse: fatal: not a git repository"), err.Error())
	assert.NotContains(t, err.Error(), "\n")
}

func TestRunGit_ErrorWithoutGitMessage(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	dir := t.TempDir()
	cmd := exec.Command(git, "init", "-q", dir)
	require.NoError(t, cmd.Run())

	// --quiet makes git fail without printing anything: the exit status is
	// all there is to report.
	_, err = runGit(context.Background(), git, dir, "rev-parse", "--verify", "--quiet", "nope")
	require.Error(t, err)
	assert.Equal(t, "git rev-parse: exit status 1", err.Error())
}
