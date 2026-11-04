package adapter

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote/errcode"
	"oras.land/oras-go/v2/registry/remote/retry"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestParseReferenceRequiresTag(t *testing.T) {
	_, err := parseReference("ghcr.io/org/repo")
	require.ErrorContains(t, err, "must include a tag")

	parsed, err := parseReference("ghcr.io/org/repo:main")
	require.NoError(t, err)
	require.Equal(t, "main", parsed.Reference)

	_, err = parseReference("not a ref")
	require.ErrorContains(t, err, "parse reference")
}

func TestParseTaggedReferenceRejectsDigest(t *testing.T) {
	_, err := parseTaggedReference("ghcr.io/org/repo@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	require.ErrorContains(t, err, "requires a tag")

	_, err = parseTaggedReference("ghcr.io/org/repo:main")
	require.NoError(t, err)
}

func TestValidateReportsDir(t *testing.T) {
	require.Error(t, validateReportsDir(filepath.Join(t.TempDir(), "missing")))

	file := filepath.Join(t.TempDir(), "file.yaml")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	require.ErrorContains(t, validateReportsDir(file), "not a directory")

	empty := t.TempDir()
	require.ErrorContains(t, validateReportsDir(empty), "empty")

	full := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(full, "_index.yaml"), []byte("x"), 0o600))
	require.NoError(t, validateReportsDir(full))
}

func TestRegistryCredentialEnv(t *testing.T) {
	t.Setenv(registryUsernameEnv, "user")
	t.Setenv(registryPasswordEnv, "")

	_, err := registryCredential("ghcr.io")
	require.ErrorContains(t, err, "is empty")

	t.Setenv(registryPasswordEnv, "token")

	cred, err := registryCredential("ghcr.io")
	require.NoError(t, err)

	got, err := cred(t.Context(), "ghcr.io")
	require.NoError(t, err)
	require.Equal(t, "user", got.Username)
	require.Equal(t, "token", got.Password)

	t.Setenv(registryUsernameEnv, "")

	cred, err = registryCredential("ghcr.io")
	require.NoError(t, err)

	got, err = cred(t.Context(), "ghcr.io")
	require.NoError(t, err)
	require.Equal(t, "token", got.AccessToken)
}

func TestInsecureRetryClientKeepsDefaultTransportSettings(t *testing.T) {
	client := insecureRetryClient()

	retrying, ok := client.Transport.(*retry.Transport)
	require.True(t, ok)

	transport, ok := retrying.Base.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.Proxy, "keeps the default transport's proxy settings")
	require.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	require.Equal(t, uint16(tls.VersionTLS12), transport.TLSClientConfig.MinVersion)
}

func TestRegistryCredentialFallsBackToDockerConfig(t *testing.T) {
	t.Setenv(registryUsernameEnv, "")
	t.Setenv(registryPasswordEnv, "")

	dockerConfig := t.TempDir()
	auth := base64.StdEncoding.EncodeToString([]byte("docker-user:docker-pass"))
	require.NoError(t, os.WriteFile(filepath.Join(dockerConfig, "config.json"),
		[]byte(`{"auths":{"registry.example.com":{"auth":"`+auth+`"}}}`), 0o600))
	t.Setenv("DOCKER_CONFIG", dockerConfig)

	cred, err := registryCredential("registry.example.com")
	require.NoError(t, err)
	require.NotNil(t, cred)

	got, err := cred(t.Context(), "registry.example.com")
	require.NoError(t, err)
	require.Equal(t, "docker-user", got.Username)
	require.Equal(t, "docker-pass", got.Password)
}

// gitlabAllowedMediaTypes copies the entries of GitLab's container registry
// allowlist that compat mode relies on:
// https://gitlab.com/gitlab-org/container-registry/-/blob/master/docs/supported-media-types.md
var gitlabAllowedMediaTypes = []string{
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.unknown.config.v1+json",
	"application/vnd.oci.empty.v1+json",
	"application/tar+gzip",
}

// packed packs a small reports archive in a file store and returns the
// manifest it tagged.
func packed(t *testing.T, compat bool) ocispec.Manifest {
	t.Helper()

	ctx := context.Background()
	staging := t.TempDir()
	archive := filepath.Join(staging, reportsBlobName)
	require.NoError(t, tarGz(reportsFixture(t), archive))

	store, err := file.New(staging)
	require.NoError(t, err)

	t.Cleanup(func() { _ = store.Close() })

	desc, err := (&ORASRegistry{}).pack(ctx, store, archive, "main", compat)
	require.NoError(t, err)

	raw, err := content.FetchAll(ctx, store, desc)
	require.NoError(t, err)

	var manifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, desc.MediaType, manifest.MediaType)

	return manifest
}

// reportsFixture returns a reports dir holding one file.
func reportsFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pkg", "report.json"), []byte(`{"ok":true}`), 0o600))

	return dir
}

func TestPack_GoozeMediaTypes(t *testing.T) {
	manifest := packed(t, false)

	assert.Equal(t, ocispec.MediaTypeImageManifest, manifest.MediaType)
	assert.Equal(t, "application/vnd.gooze.reports.v1+gzip", manifest.ArtifactType)
	assert.Equal(t, ocispec.MediaTypeEmptyJSON, manifest.Config.MediaType)
	require.Len(t, manifest.Layers, 1)
	assert.Equal(t, "application/gzip", manifest.Layers[0].MediaType)
	assert.Equal(t, reportsBlobName, manifest.Layers[0].Annotations[ocispec.AnnotationTitle])
}

func TestPack_Compat(t *testing.T) {
	manifest := packed(t, true)

	assert.Equal(t, ocispec.MediaTypeImageManifest, manifest.MediaType)
	assert.Empty(t, manifest.ArtifactType)
	assert.Equal(t, "application/vnd.unknown.config.v1+json", manifest.Config.MediaType)
	require.Len(t, manifest.Layers, 1)
	assert.Equal(t, "application/tar+gzip", manifest.Layers[0].MediaType)
	assert.Equal(t, "1", manifest.Layers[0].Annotations["dev.gooze.reports.version"])
	assert.Equal(t, reportsBlobName, manifest.Layers[0].Annotations[ocispec.AnnotationTitle])
}

func TestPack_CompatUsesOnlyGitLabMediaTypes(t *testing.T) {
	manifest := packed(t, true)

	used := []string{manifest.MediaType, manifest.Config.MediaType}
	for _, layer := range manifest.Layers {
		used = append(used, layer.MediaType)
	}

	for _, mediaType := range used {
		assert.Contains(t, gitlabAllowedMediaTypes, mediaType)
	}
}

func TestPushPull_RoundTrip(t *testing.T) {
	for _, compat := range []bool{false, true} {
		t.Run(fmt.Sprintf("compat=%v", compat), func(t *testing.T) {
			ctx := context.Background()
			registry := memory.New()

			require.NoError(t, pushTo(ctx, reportsFixture(t), registry, "main", compat))

			dest := t.TempDir()
			require.NoError(t, pullFrom(ctx, registry, "main", dest))

			got, err := os.ReadFile(filepath.Join(dest, "pkg", "report.json"))
			require.NoError(t, err)
			assert.JSONEq(t, `{"ok":true}`, string(got))
		})
	}
}

func TestPull_PicksTheReportsLayer(t *testing.T) {
	plain := ocispec.Descriptor{MediaType: "application/octet-stream"}
	gzipLayer := ocispec.Descriptor{MediaType: "application/gzip"}
	tarGzip := ocispec.Descriptor{MediaType: "application/tar+gzip"}
	annotated := ocispec.Descriptor{MediaType: "application/octet-stream", Annotations: map[string]string{"dev.gooze.reports.version": "1"}}

	for _, tt := range []struct {
		name   string
		layers []ocispec.Descriptor
		want   ocispec.Descriptor
		ok     bool
	}{
		{name: "gooze media type", layers: []ocispec.Descriptor{plain, gzipLayer}, want: gzipLayer, ok: true},
		{name: "compat media type", layers: []ocispec.Descriptor{plain, tarGzip}, want: tarGzip, ok: true},
		{name: "annotation first", layers: []ocispec.Descriptor{tarGzip, annotated}, want: annotated, ok: true},
		{name: "none", layers: []ocispec.Descriptor{plain}},
		{name: "media types that only sort near ours", layers: []ocispec.Descriptor{
			{MediaType: "application/a+json"}, {MediaType: "application/gzip-other"}, {MediaType: "application/zip"},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := reportsLayer(tt.layers)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPull_RejectsAnArtifactWithoutReports(t *testing.T) {
	ctx := context.Background()
	registry := memory.New()

	layer, err := oras.PushBytes(ctx, registry, "application/octet-stream", []byte("not reports"))
	require.NoError(t, err)

	manifest, err := oras.PackManifest(ctx, registry, oras.PackManifestVersion1_1, "application/vnd.example", oras.PackManifestOptions{Layers: []ocispec.Descriptor{layer}})
	require.NoError(t, err)
	require.NoError(t, registry.Tag(ctx, manifest, "main"))

	require.ErrorContains(t, pullFrom(ctx, registry, "main", t.TempDir()), "not a gooze reports artifact")
}

func TestCompatHint(t *testing.T) {
	rejected := &errcode.ErrorResponse{
		Method:     http.MethodPut,
		URL:        &url.URL{Scheme: "https", Host: "registry.gitlab.com", Path: "/v2/g/p/manifests/master"},
		StatusCode: http.StatusBadRequest,
		Errors: errcode.Errors{{
			Code: errcode.ErrorCodeManifestInvalid, Message: "manifest invalid",
			Detail: "unknown media type: application/vnd.gooze.reports.v1+gzip",
		}},
	}

	err := compatHint(fmt.Errorf("push: %w", rejected))
	require.ErrorIs(t, err, rejected)
	assert.ErrorContains(t, err, "report.oci.compat=true")
	assert.ErrorContains(t, err, "GOOZE_REPORT_OCI_COMPAT=true")
	assert.ErrorContains(t, err, "supported-media-types.md")

	for _, status := range []int{http.StatusFound, http.StatusUnauthorized} {
		other := &errcode.ErrorResponse{StatusCode: status, URL: &url.URL{}, Errors: rejected.Errors}
		assert.Equal(t, other, compatHint(other), status)
	}

	otherBadRequest := &errcode.ErrorResponse{
		StatusCode: http.StatusBadRequest, URL: &url.URL{},
		Errors: errcode.Errors{{Code: errcode.ErrorCodeNameInvalid, Message: "invalid repository name"}},
	}
	assert.Equal(t, otherBadRequest, compatHint(otherBadRequest))

	boom := errors.New("boom")
	assert.Equal(t, boom, compatHint(boom))
	assert.NoError(t, compatHint(nil))
}

// gitlabLikeRegistry serves the upload half of the OCI distribution API and
// rejects, as GitLab's registry does, a manifest that uses a media type off
// gitlabAllowedMediaTypes. It returns the host and the tags it stored.
func gitlabLikeRegistry(t *testing.T) (string, map[string]bool) {
	t.Helper()

	tags := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodHead && strings.Contains(req.URL.Path, "/blobs/"):
			w.WriteHeader(http.StatusNotFound)
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/blobs/uploads/"):
			w.Header().Set("Location", req.URL.Path+"upload")
			w.WriteHeader(http.StatusAccepted)
		case req.Method == http.MethodPut && strings.Contains(req.URL.Path, "/blobs/uploads/"):
			_, _ = io.Copy(io.Discard, req.Body)
			w.WriteHeader(http.StatusCreated)
		case req.Method == http.MethodPut && strings.Contains(req.URL.Path, "/manifests/"):
			var manifest ocispec.Manifest
			require.NoError(t, json.NewDecoder(req.Body).Decode(&manifest))

			used := []string{manifest.MediaType, manifest.Config.MediaType}
			if manifest.ArtifactType != "" {
				used = append(used, manifest.ArtifactType)
			}

			for _, layer := range manifest.Layers {
				used = append(used, layer.MediaType)
			}

			for _, mediaType := range used {
				if !slices.Contains(gitlabAllowedMediaTypes, mediaType) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = fmt.Fprintf(w, `{"errors":[{"code":"MANIFEST_INVALID","message":"manifest invalid","detail":"unknown media type: %s"}]}`, mediaType)

					return
				}
			}

			tags[req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]] = true
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	return strings.TrimPrefix(srv.URL, "http://"), tags
}

func TestPush_GitLabLikeRegistry(t *testing.T) {
	t.Setenv(registryUsernameEnv, "")
	t.Setenv(registryPasswordEnv, "token")

	host, tags := gitlabLikeRegistry(t)
	ref := host + "/group/project/gooze-reports:master"
	reports := m.Path(reportsFixture(t))

	err := NewORASRegistry().Push(context.Background(), ref, reports, domain.RegistryOptions{PlainHTTP: true})
	require.ErrorContains(t, err, "unknown media type: application/vnd.gooze.reports.v1+gzip")
	require.ErrorContains(t, err, "report.oci.compat=true")
	assert.Empty(t, tags)

	require.NoError(t, NewORASRegistry().Push(context.Background(), ref, reports, domain.RegistryOptions{PlainHTTP: true, OCICompat: true}))
	assert.True(t, tags["master"])
}

func TestPushPull_RegistryErrors(t *testing.T) {
	t.Setenv(registryUsernameEnv, "")
	t.Setenv(registryPasswordEnv, "token")

	host, _ := gitlabLikeRegistry(t)
	reports := m.Path(reportsFixture(t))
	plain := domain.RegistryOptions{PlainHTTP: true}

	// The fake serves no manifests, so a pull finds no tag.
	err := NewORASRegistry().Pull(context.Background(), host+"/group/project:missing", m.Path(t.TempDir()), plain)
	require.ErrorContains(t, err, "pull from")

	_, err = (&ORASRegistry{}).repository(registry.Reference{Registry: "bad host", Repository: "r"}, plain)
	require.ErrorContains(t, err, "create repository client")

	require.ErrorContains(t, NewORASRegistry().Push(context.Background(), "bad host/r:main", reports, plain), "parse reference")
	require.ErrorContains(t, NewORASRegistry().Pull(context.Background(), "bad host/r:main", reports, plain), "parse reference")

	t.Setenv(registryUsernameEnv, "user")
	t.Setenv(registryPasswordEnv, "")

	require.ErrorContains(t, NewORASRegistry().Push(context.Background(), host+"/g/p:main", reports, plain), "is empty")
	require.ErrorContains(t, NewORASRegistry().Pull(context.Background(), host+"/g/p:main", reports, plain), "is empty")
}
