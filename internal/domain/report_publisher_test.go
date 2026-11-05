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
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestReportPublisher_Push(t *testing.T) {
	reg := domainmocks.NewMockOCIRegistry(t)
	reg.On("Push", mock.Anything, "ref:tag", m.Path("/r"), domain.RegistryOptions{PlainHTTP: true}).Return(nil)

	pub := domain.NewReportPublisher(reg, domainmocks.NewMockReportStore(t))

	require.NoError(t, pub.Push(context.Background(), domain.PushArgs{Reports: "/r", Ref: "ref:tag", PlainHTTP: true}))
	reg.AssertExpectations(t)
}

func TestReportPublisher_Pull(t *testing.T) {
	reg := domainmocks.NewMockOCIRegistry(t)
	reg.On("Pull", mock.Anything, "ref:tag", m.Path("/r"), domain.RegistryOptions{Insecure: true}).Return("sha256:abc", nil)

	source := rpt.Source{Origin: &rpt.File{ShortPath: "a.go"}}
	store := domainmocks.NewMockReportStore(t)
	store.EXPECT().LoadSpillReports(mock.Anything, m.Path("/r")).Return(spillOf(t,
		m.Report{Source: source, Result: m.Result{mgen.Arithmetic: {{MutationID: "1", Status: rpt.Killed}}}},
		m.Report{Source: source, Result: m.Result{mgen.Comparison: {{MutationID: "2", Status: rpt.Survived}}}},
	), nil)

	pub := domain.NewReportPublisher(reg, store)

	got, err := pub.Pull(context.Background(), domain.PullArgs{Reports: "/r", Ref: "ref:tag", Insecure: true})
	require.NoError(t, err)
	assert.Equal(t, "sha256:abc", got.Digest)
	assert.Equal(t, uint64(2), got.Reports)
	assert.Equal(t, 1, got.Score.Killed)
	assert.Equal(t, 1, got.Score.Survived)
}

func TestReportPublisher_Validation(t *testing.T) {
	pub := domain.NewReportPublisher(domainmocks.NewMockOCIRegistry(t), domainmocks.NewMockReportStore(t))

	err := pub.Push(context.Background(), domain.PushArgs{Reports: "/r"}) // missing ref
	require.Error(t, err)
	assert.Equal(t, m.ExitUsage, exitCodeOf(err))
	require.Error(t, pub.Push(context.Background(), domain.PushArgs{Ref: "x"})) // missing reports
	_, err = pub.Pull(context.Background(), domain.PullArgs{Reports: "/r"})     // missing ref
	require.Error(t, err)
}

func TestReportPublisher_PropagatesError(t *testing.T) {
	reg := domainmocks.NewMockOCIRegistry(t)
	boom := errors.New("boom")
	reg.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(boom)

	pub := domain.NewReportPublisher(reg, domainmocks.NewMockReportStore(t))

	err := pub.Push(context.Background(), domain.PushArgs{Reports: "/r", Ref: "x"})
	require.ErrorIs(t, err, boom)
	assert.Equal(t, m.ExitRegistry, exitCodeOf(err))

	reg.On("Pull", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", boom)
	_, err = pub.Pull(context.Background(), domain.PullArgs{Reports: "/r", Ref: "x"})
	assert.Equal(t, m.ExitRegistry, exitCodeOf(err))
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

			pub := domain.NewReportPublisher(reg, domainmocks.NewMockReportStore(t))

			require.NoError(t, pub.Push(context.Background(), domain.PushArgs{
				Reports: "/r", Ref: tt.ref, OCICompat: tt.force, OCICompatHosts: tt.hosts,
			}))
		})
	}
}
