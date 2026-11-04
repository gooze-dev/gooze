package adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type envRecorder struct{ env []string }

func (r *envRecorder) SetBuildEnv(env []string) { r.env = env }

func TestBuildEnvs_SetsEveryOne(t *testing.T) {
	a, b := &envRecorder{}, &envRecorder{}

	BuildEnvs{a, b}.SetBuildEnv([]string{"GOCACHEPROG=x"})

	assert.Equal(t, []string{"GOCACHEPROG=x"}, a.env)
	assert.Equal(t, []string{"GOCACHEPROG=x"}, b.env)
}
