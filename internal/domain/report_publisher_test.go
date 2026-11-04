package domain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestReportPublisher_Push(t *testing.T) {
	reg := domainmocks.NewMockOCIRegistry(t)
	reg.On("Push", mock.Anything, "ref:tag", m.Path("/r"), domain.RegistryOptions{PlainHTTP: true}).Return(nil)

	pub := domain.NewReportPublisher(reg)

	require.NoError(t, pub.Push(context.Background(), domain.PushArgs{Reports: "/r", Ref: "ref:tag", PlainHTTP: true}))
	reg.AssertExpectations(t)
}

func TestReportPublisher_Pull(t *testing.T) {
	reg := domainmocks.NewMockOCIRegistry(t)
	reg.On("Pull", mock.Anything, "ref:tag", m.Path("/r"), domain.RegistryOptions{Insecure: true}).Return(nil)

	pub := domain.NewReportPublisher(reg)

	require.NoError(t, pub.Pull(context.Background(), domain.PullArgs{Reports: "/r", Ref: "ref:tag", Insecure: true}))
	reg.AssertExpectations(t)
}

func TestReportPublisher_Validation(t *testing.T) {
	pub := domain.NewReportPublisher(domainmocks.NewMockOCIRegistry(t))

	err := pub.Push(context.Background(), domain.PushArgs{Reports: "/r"}) // missing ref
	require.Error(t, err)
	assert.Equal(t, m.ExitUsage, exitCodeOf(err))
	require.Error(t, pub.Push(context.Background(), domain.PushArgs{Ref: "x"}))      // missing reports
	require.Error(t, pub.Pull(context.Background(), domain.PullArgs{Reports: "/r"})) // missing ref
}

func TestReportPublisher_PropagatesError(t *testing.T) {
	reg := domainmocks.NewMockOCIRegistry(t)
	boom := errors.New("boom")
	reg.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(boom)

	pub := domain.NewReportPublisher(reg)

	err := pub.Push(context.Background(), domain.PushArgs{Reports: "/r", Ref: "x"})
	require.ErrorIs(t, err, boom)
	assert.Equal(t, m.ExitRegistry, exitCodeOf(err))

	reg.On("Pull", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(boom)
	assert.Equal(t, m.ExitRegistry, exitCodeOf(pub.Pull(context.Background(), domain.PullArgs{Reports: "/r", Ref: "x"})))
}

func TestReportPublisher_PushOCICompat(t *testing.T) {
	on, off := true, false

	tests := []struct {
		name  string
		ref   string
		force *bool
		hosts []string
		want  bool
	}{
		{name: "ghcr keeps the gooze media types", ref: "ghcr.io/org/repo:main"},
		{name: "docker hub keeps the gooze media types", ref: "docker.io/org/repo:main"},
		{name: "gitlab.com registry", ref: "registry.gitlab.com/group/project/gooze-reports:master", want: true},
		{name: "gitlab.com registry, any case and port", ref: "Registry.GitLab.com:443/group/project:master", want: true},
		{name: "forced", ref: "ghcr.io/org/repo:main", force: &on, want: true},
		{name: "turned off for gitlab.com", ref: "registry.gitlab.com/group/project:master", force: &off},
		{name: "turned off for a listed host", ref: "gitlab.example.com/g/p:main", force: &off, hosts: []string{"gitlab.example.com"}},
		{name: "listed host", ref: "gitlab.example.com/group/project:main", hosts: []string{"gitlab.example.com"}, want: true},
		{name: "subdomain of a listed host", ref: "registry.gitlab.example.com/group/project:main", hosts: []string{"gitlab.example.com"}, want: true},
		{name: "comma-separated hosts from the environment", ref: "b.example.com/p:main", hosts: []string{"a.example.com, b.example.com"}, want: true},
		{name: "only a name suffix, not a subdomain", ref: "evilgitlab.example.com/p:main", hosts: []string{"gitlab.example.com"}},
		{name: "path that names gitlab", ref: "ghcr.io/registry.gitlab.com/p:main"},
		{name: "host that sorts after a listed one", ref: "zot.internal/p:main", hosts: []string{"gitlab.example.com"}},
		{name: "empty entries match nothing", ref: "ghcr.io./org/repo:main", hosts: []string{"", " , "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := domainmocks.NewMockOCIRegistry(t)
			reg.On("Push", mock.Anything, tt.ref, m.Path("/r"), domain.RegistryOptions{OCICompat: tt.want}).Return(nil)

			pub := domain.NewReportPublisher(reg)

			require.NoError(t, pub.Push(context.Background(), domain.PushArgs{
				Reports: "/r", Ref: tt.ref, OCICompat: tt.force, OCICompatHosts: tt.hosts,
			}))
		})
	}
}
