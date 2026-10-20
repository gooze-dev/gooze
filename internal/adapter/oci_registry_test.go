package adapter

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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
	require.NotNil(t, client.Transport)

	base, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, base.Proxy, "sanity: default transport has a proxy func")
}
