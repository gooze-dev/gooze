package adapter

import "gooze.dev/pkg/gooze/internal/domain"

// BuildEnvs passes a build environment on to each of its go command runners:
// the test runner and the type checks behind mutant generation.
type BuildEnvs []domain.BuildEnvironment

// SetBuildEnv implements domain.BuildEnvironment.
func (envs BuildEnvs) SetBuildEnv(env []string) {
	for _, e := range envs {
		e.SetBuildEnv(env)
	}
}
