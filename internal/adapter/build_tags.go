package adapter

import (
	"go/build"
	"os"
	"path/filepath"
	"strings"

	"gooze.dev/pkg/gooze/internal/domain"
)

// buildTags are the build tags that select the files of a package: the ones
// a run sets, or else the -tags in GOFLAGS.
type buildTags struct {
	tags []string // nil: follow GOFLAGS
}

// SetBuildTags implements domain.BuildTags.
func (b *buildTags) SetBuildTags(tags []string) {
	b.tags = tags
}

// context returns the build context that selects files as go test does.
func (b *buildTags) context() build.Context {
	ctxt := build.Default

	ctxt.BuildTags = b.tags
	if b.tags == nil {
		ctxt.BuildTags = goflagsTags(os.Getenv("GOFLAGS"))
	}

	return ctxt
}

// excludes reports whether go test leaves path out of its package: its file
// name or //go:build line doesn't match GOOS, GOARCH and the tags. Its
// mutants could never be compiled.
func (b *buildTags) excludes(path string) bool {
	ctxt := b.context()
	match, err := ctxt.MatchFile(filepath.Dir(path), filepath.Base(path))

	return err == nil && !match
}

// goArgs returns args with the -tags flag of the tags a run set, for a go
// command that must select the same files. GOFLAGS reaches it by itself.
func (b *buildTags) goArgs(args ...string) []string {
	if b.tags == nil {
		return args
	}

	return append(args, "-tags="+strings.Join(b.tags, ","))
}

// goflagsTags returns the tags of the last -tags flag in goflags.
func goflagsTags(goflags string) []string {
	var tags []string

	for _, flag := range strings.Fields(goflags) {
		if name, value, _ := strings.Cut(strings.TrimLeft(flag, "-"), "="); name == "tags" {
			tags = strings.Split(value, ",")
		}
	}

	return tags
}

// BuildTagSetters passes build tags on to each of its file selectors: the
// source scan and the type checks behind mutant generation.
type BuildTagSetters []domain.BuildTags

// SetBuildTags implements domain.BuildTags.
func (setters BuildTagSetters) SetBuildTags(tags []string) {
	for _, s := range setters {
		s.SetBuildTags(tags)
	}
}
