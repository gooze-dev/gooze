package domain

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// UpdateCommand is how users upgrade gooze; the update notice points at it.
const UpdateCommand = "go install gooze.dev/pkg/gooze@latest"

// ReleaseChecker reports the tag of the latest published gooze release (e.g.
// "v1.3.0"). It is split so the main flow never waits on the network: Cached
// answers from local state only, and Refresh, which may be slow or fail when
// offline, is meant to run in the background. An empty tag means unknown.
type ReleaseChecker interface {
	// Cached returns the last known tag and whether it is fresh enough that no
	// Refresh is needed.
	Cached() (tag string, fresh bool)
	// Refresh looks up the latest tag and records it for later Cached calls.
	Refresh(ctx context.Context) (string, error)
}

// UpdateNotice describes a newer release than the running build.
type UpdateNotice struct {
	Current string
	Latest  string
}

// String renders the notice as a single plain-text line.
func (n UpdateNotice) String() string {
	return fmt.Sprintf("gooze %s is available (you have %s). Update: %s", n.Latest, n.Current, UpdateCommand)
}

// Released reports whether v is a release or pseudo-version that can be
// compared, rather than an unknown or "(devel)" build.
func Released(v string) bool {
	_, ok := parseSemver(v)

	return ok
}

// NoticeFor returns a notice when latest is newer than current, and false for
// an unknown latest, an up-to-date build or an unreleased one.
func NoticeFor(current, latest string) (UpdateNotice, bool) {
	if !IsNewerVersion(latest, current) {
		return UpdateNotice{}, false
	}

	return UpdateNotice{Current: current, Latest: latest}, true
}

// IsNewerVersion reports whether candidate is a higher semantic version than
// current. Both are "vMAJOR.MINOR.PATCH" with an optional pre-release and build
// suffix; anything unparsable is never newer.
func IsNewerVersion(candidate, current string) bool {
	c, ok := parseSemver(candidate)
	if !ok {
		return false
	}

	cur, ok := parseSemver(current)
	if !ok {
		return false
	}

	return compareSemver(c, cur) > 0
}

type semver struct {
	core [3]int
	pre  string
}

func parseSemver(v string) (semver, bool) {
	rest, ok := strings.CutPrefix(v, "v")
	if !ok {
		return semver{}, false
	}

	rest, _, _ = strings.Cut(rest, "+")
	rest, pre, _ := strings.Cut(rest, "-")

	parts := strings.Split(rest, ".")
	if len(parts) != len(semver{}.core) {
		return semver{}, false
	}

	var s semver

	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}

		s.core[i] = n
	}

	s.pre = pre

	return s, true
}

// compareSemver orders by the numeric core, then ranks a release above any of
// its pre-releases, so a pseudo-version like v1.2.1-0.20260101-abcdef (a commit
// after v1.2.0) sorts above v1.2.0 and below v1.2.1. Pre-releases compare
// lexically, which is enough to rank them against the latest release tag.
func compareSemver(a, b semver) int {
	for i := range a.core {
		if a.core[i] != b.core[i] {
			if a.core[i] > b.core[i] {
				return 1
			}

			return -1
		}
	}

	switch {
	case a.pre == b.pre:
		return 0
	case a.pre == "":
		return 1
	case b.pre == "":
		return -1
	}

	return strings.Compare(a.pre, b.pre)
}
