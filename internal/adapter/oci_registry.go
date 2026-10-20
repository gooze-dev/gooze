package adapter

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"

	m "gooze.dev/pkg/gooze/internal/model"
)

const (
	registryUsernameEnv = "GOOZE_REGISTRY_USERNAME"
	registryPasswordEnv = "GOOZE_REGISTRY_PASSWORD" //nolint:gosec // env var name, not a secret.
	// reportsArtifactType identifies a Gooze reports artifact in a registry.
	reportsArtifactType = "application/vnd.gooze.reports.v1+gzip"
	// reportsBlobMediaType is the media type of the packaged reports blob.
	reportsBlobMediaType = "application/gzip"
	// reportsBlobName is the file name of the packaged reports blob.
	reportsBlobName = "gooze-reports.tgz"
)

// RegistryOptions tweaks how the registry is contacted.
type RegistryOptions struct {
	PlainHTTP bool // use http instead of https
	Insecure  bool // skip TLS certificate verification
}

// OCIRegistry stores and retrieves a reports directory as an OCI artifact.
type OCIRegistry interface {
	Push(ctx context.Context, ref string, dir m.Path, opts RegistryOptions) error
	Pull(ctx context.Context, ref string, dir m.Path, opts RegistryOptions) error
}

// ORASRegistry implements OCIRegistry using oras-go against any OCI registry.
type ORASRegistry struct{}

// NewORASRegistry constructs an ORAS-backed OCIRegistry.
func NewORASRegistry() *ORASRegistry {
	return &ORASRegistry{}
}

// Push packages dir into a gzip blob and pushes it to ref as an OCI artifact.
func (r *ORASRegistry) Push(ctx context.Context, ref string, dir m.Path, opts RegistryOptions) error {
	parsed, err := parseTaggedReference(ref)
	if err != nil {
		return err
	}

	if err := validateReportsDir(string(dir)); err != nil {
		return err
	}

	staging, err := os.MkdirTemp("", "gooze-oci-push-*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}

	defer func() { _ = os.RemoveAll(staging) }()

	archivePath := filepath.Join(staging, reportsBlobName)
	if err := tarGz(string(dir), archivePath); err != nil {
		return fmt.Errorf("package reports: %w", err)
	}

	store, err := file.New(staging)
	if err != nil {
		return fmt.Errorf("create file store: %w", err)
	}

	defer func() { _ = store.Close() }()

	tag := parsed.Reference

	if err := r.pack(ctx, store, archivePath, tag); err != nil {
		return err
	}

	repo, err := r.repository(parsed, opts)
	if err != nil {
		return err
	}

	if _, err := oras.Copy(ctx, store, tag, repo, tag, oras.DefaultCopyOptions); err != nil {
		return fmt.Errorf("push to %q: %w", ref, err)
	}

	return nil
}

// Pull fetches the artifact at ref and extracts its reports into dir.
func (r *ORASRegistry) Pull(ctx context.Context, ref string, dir m.Path, opts RegistryOptions) error {
	parsed, err := parseReference(ref)
	if err != nil {
		return err
	}

	staging, err := os.MkdirTemp("", "gooze-oci-pull-*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}

	defer func() { _ = os.RemoveAll(staging) }()

	store, err := file.New(staging)
	if err != nil {
		return fmt.Errorf("create file store: %w", err)
	}

	defer func() { _ = store.Close() }()

	repo, err := r.repository(parsed, opts)
	if err != nil {
		return err
	}

	if _, err := oras.Copy(ctx, repo, parsed.Reference, store, parsed.Reference, oras.DefaultCopyOptions); err != nil {
		return fmt.Errorf("pull from %q: %w", ref, err)
	}

	archivePath := filepath.Join(staging, reportsBlobName)
	if _, err := os.Stat(archivePath); err != nil {
		return fmt.Errorf("artifact at %q is not a gooze reports artifact (missing %s layer)", ref, reportsBlobName)
	}

	if err := unTarGz(archivePath, string(dir)); err != nil {
		return fmt.Errorf("unpack reports: %w", err)
	}

	return nil
}

// parseReference parses ref and requires a tag or digest.
func parseReference(ref string) (registry.Reference, error) {
	parsed, err := registry.ParseReference(ref)
	if err != nil {
		return registry.Reference{}, fmt.Errorf("parse reference %q: %w", ref, err)
	}

	if parsed.Reference == "" {
		return registry.Reference{}, fmt.Errorf("reference %q must include a tag (for example %s:main)", ref, ref)
	}

	return parsed, nil
}

// parseTaggedReference parses ref and requires a tag.
func parseTaggedReference(ref string) (registry.Reference, error) {
	parsed, err := parseReference(ref)
	if err != nil {
		return registry.Reference{}, err
	}

	if parsed.ValidateReferenceAsDigest() == nil {
		return registry.Reference{}, fmt.Errorf("push requires a tag, got digest reference %q", ref)
	}

	return parsed, nil
}

// validateReportsDir checks that dir is a non-empty directory.
func validateReportsDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("reports dir %q: %w", dir, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("reports dir %q is not a directory", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reports dir %q: %w", dir, err)
	}

	if len(entries) == 0 {
		return fmt.Errorf("reports dir %q is empty; nothing to push", dir)
	}

	return nil
}

// pack adds the archive as a layer and tags a wrapping artifact manifest.
func (r *ORASRegistry) pack(ctx context.Context, store *file.Store, archivePath, tag string) error {
	layer, err := store.Add(ctx, reportsBlobName, reportsBlobMediaType, archivePath)
	if err != nil {
		return fmt.Errorf("add reports blob: %w", err)
	}

	manifest, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, reportsArtifactType, oras.PackManifestOptions{
		Layers: []ocispec.Descriptor{layer},
	})
	if err != nil {
		return fmt.Errorf("pack manifest: %w", err)
	}

	if err := store.Tag(ctx, manifest, tag); err != nil {
		return fmt.Errorf("tag manifest: %w", err)
	}

	return nil
}

// repository builds an authenticated remote repository client for ref.
func (r *ORASRegistry) repository(ref registry.Reference, opts RegistryOptions) (*remote.Repository, error) {
	repo, err := remote.NewRepository(fmt.Sprintf("%s/%s", ref.Registry, ref.Repository))
	if err != nil {
		return nil, fmt.Errorf("create repository client: %w", err)
	}

	repo.PlainHTTP = opts.PlainHTTP

	client, err := registryClient(ref.Registry, opts)
	if err != nil {
		return nil, err
	}

	repo.Client = client

	return repo, nil
}

// registryClient returns an auth client using env credentials when set, falling
// back to the Docker credential store.
func registryClient(host string, opts RegistryOptions) (*auth.Client, error) {
	httpClient := retry.DefaultClient
	if opts.Insecure {
		httpClient = insecureRetryClient()
	}

	credential, err := registryCredential(host)
	if err != nil {
		return nil, err
	}

	return &auth.Client{
		Client:     httpClient,
		Cache:      auth.NewCache(),
		Credential: credential,
	}, nil
}

func registryCredential(host string) (auth.CredentialFunc, error) {
	user, password := os.Getenv(registryUsernameEnv), os.Getenv(registryPasswordEnv)

	switch {
	case user != "" && password != "":
		return auth.StaticCredential(host, auth.Credential{Username: user, Password: password}), nil
	case user != "":
		return nil, fmt.Errorf("%s is set but %s is empty", registryUsernameEnv, registryPasswordEnv)
	case password != "":
		return auth.StaticCredential(host, auth.Credential{AccessToken: password}), nil
	}

	store, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
	if err != nil {
		return nil, fmt.Errorf("load docker credential store: %w", err)
	}

	return credentials.Credential(store), nil
}

// insecureRetryClient mirrors retry.DefaultClient but skips TLS verification.
func insecureRetryClient() *http.Client {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		transport = &http.Transport{}
	} else {
		transport = transport.Clone()
	}

	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // --insecure is an explicit opt-in.

	return &http.Client{Transport: retry.NewTransport(transport)}
}
