package gocache

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// minGoMinor is the first Go 1.x release with GOCACHEPROG.
const minGoMinor = 24

var goMinor = regexp.MustCompile(`^(?:devel )?go1\.(\d+)`)

type goEnv struct {
	GOVERSION, GOCACHE, GOCACHEPROG string
}

func readGoEnv(ctx context.Context) (goEnv, error) {
	out, err := exec.CommandContext(ctx, "go", "env", "-json", "GOVERSION", "GOCACHE", "GOCACHEPROG").Output()
	if err != nil {
		return goEnv{}, fmt.Errorf("go env: %w", err)
	}

	var env goEnv

	return env, json.Unmarshal(out, &env)
}

// fallbackReason says why mutants can't use gooze's cache, or is empty.
func fallbackReason(env goEnv) string {
	switch {
	case env.GOCACHEPROG != "":
		return "GOCACHEPROG is already set"
	case !supportsCacheProg(env.GOVERSION):
		return "GOCACHEPROG needs Go 1.24 or later, not " + env.GOVERSION
	default:
		return ""
	}
}

func supportsCacheProg(version string) bool {
	match := goMinor.FindStringSubmatch(version)
	if match == nil {
		return false
	}

	minor, err := strconv.Atoi(match[1])

	return err == nil && minor >= minGoMinor
}

// progCommand returns the GOCACHEPROG value that runs gooze's server, its
// arguments quoted as the go command splits them.
func progCommand(exe, shared, layerDir string) string {
	quote := func(s string) string {
		if strings.Contains(s, `"`) {
			return "'" + s + "'"
		}

		return `"` + s + `"`
	}

	return quote(exe) + " __cacheprog --shared " + quote(shared) + " --layer " + quote(layerDir)
}
