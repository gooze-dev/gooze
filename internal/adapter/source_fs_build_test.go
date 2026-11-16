package adapter

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	m "gooze.dev/pkg/gooze/internal/model"
)

func writeBuildConstraintModule(t *testing.T) (string, string) {
	t.Helper()

	otherOS := "windows"
	if runtime.GOOS == otherOS {
		otherOS = "linux"
	}

	root := t.TempDir()
	files := map[string]string{
		"go.mod":                "module p\n",
		"a.go":                  "package p\n\nfunc Strict() bool { return strict }\n",
		"strict.go":             "//go:build strict\n\npackage p\n\nconst strict = true\n",
		"loose.go":              "//go:build !strict\n\npackage p\n\nconst strict = false\n",
		"os_" + otherOS + ".go": "package p\n\nconst osName = 1\n",
		"a_test.go":             "package p\n",
	}

	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(content), 0o644))
	}

	return root, "os_" + otherOS + ".go"
}

func sourceNames(t *testing.T, roots ...string) []string {
	t.Helper()

	paths := make([]m.Path, 0, len(roots))
	for _, root := range roots {
		paths = append(paths, m.Path(root))
	}

	sources, err := NewLocalSourceFSAdapter().Get(context.Background(), paths)
	require.NoError(t, err)

	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, filepath.Base(source.Origin.FullPath))
	}

	return names
}

func TestLocalSourceFSAdapter_SkipsFilesExcludedByBuildConstraints(t *testing.T) {
	t.Setenv("GOFLAGS", "")

	root, otherOS := writeBuildConstraintModule(t)

	names := sourceNames(t, root)
	assert.ElementsMatch(t, []string{"a.go", "loose.go"}, names)
	assert.NotContains(t, names, otherOS)

	assert.Empty(t, sourceNames(t, filepath.Join(root, "strict.go")), "an excluded file given as the root is skipped too")
}

func TestLocalSourceFSAdapter_BuildConstraintsHonourGOFLAGSTags(t *testing.T) {
	t.Setenv("GOFLAGS", "-mod=mod --tags=other,strict")

	root, _ := writeBuildConstraintModule(t)

	assert.ElementsMatch(t, []string{"a.go", "strict.go"}, sourceNames(t, root))
}

func TestGoflagsTags(t *testing.T) {
	assert.Nil(t, goflagsTags(""))
	assert.Nil(t, goflagsTags("-mod=mod -count=1 -trimpath"))
	assert.Equal(t, []string{"a", "b"}, goflagsTags("-tags=a,b"))
	assert.Equal(t, []string{"c"}, goflagsTags("--tags=a -tags=c"), "the last -tags wins, as in go")
}

func TestExcludedFromBuild_UnreadableFileIsNotExcluded(t *testing.T) {
	assert.False(t, (&buildTags{}).excludes(filepath.Join(t.TempDir(), "missing_"+runtime.GOOS+".go")))
}

func TestLocalSourceFSAdapter_SetBuildTagsReplaceGOFLAGSTags(t *testing.T) {
	root, _ := writeBuildConstraintModule(t)
	names := func(a *LocalSourceFSAdapter) []string {
		sources, err := a.Get(context.Background(), []m.Path{m.Path(root)})
		require.NoError(t, err)

		names := make([]string, 0, len(sources))
		for _, source := range sources {
			names = append(names, filepath.Base(source.Origin.FullPath))
		}

		return names
	}

	t.Setenv("GOFLAGS", "")

	a := NewLocalSourceFSAdapter()
	a.SetBuildTags([]string{"other", "strict"})
	assert.ElementsMatch(t, []string{"a.go", "strict.go"}, names(a), "the tags select the files they enable and drop the ones they disable")

	t.Setenv("GOFLAGS", "-tags=strict")

	a.SetBuildTags([]string{})
	assert.ElementsMatch(t, []string{"a.go", "loose.go"}, names(a), "tags that were set win over GOFLAGS, as a go test flag does, even when empty")

	a.SetBuildTags(nil)
	assert.ElementsMatch(t, []string{"a.go", "strict.go"}, names(a), "without tags of its own it follows GOFLAGS")
}
