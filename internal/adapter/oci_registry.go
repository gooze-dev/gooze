package adapter

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/errcode"
	"oras.land/oras-go/v2/registry/remote/retry"

	"gooze.dev/pkg/gooze/internal/domain"
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
	// compatBlobMediaType is the reports blob's media type in compat mode,
	// where the manifest has no artifactType and an unknown config: GitLab's
	// registry accepts only allowlisted media types, and these are on it.
	compatBlobMediaType = "application/tar+gzip"
	// reportsVersionAnnotation marks the reports layer of a compat artifact.
	reportsVersionAnnotation = "dev.gooze.reports.version"
	reportsVersion           = "1"
	// supportedMediaTypesDoc lists the media types GitLab's registry accepts.
	supportedMediaTypesDoc = "https://gitlab.com/gitlab-org/container-registry/-/blob/master/docs/supported-media-types.md"
)

// ORASRegistry implements OCIRegistry using oras-go against any OCI registry.
type ORASRegistry struct{}

// NewORASRegistry constructs an ORAS-backed OCIRegistry.
func NewORASRegistry() *ORASRegistry {
	return &ORASRegistry{}
}

// Push packages dir into a gzip blob and pushes it to ref as an OCI artifact.
func (r *ORASRegistry) Push(ctx context.Context, ref string, dir m.Path, opts domain.RegistryOptions) error {
	parsed, err := parseTaggedReference(ref)
	if err != nil {
		return err
	}

	if err := validateReportsDir(string(dir)); err != nil {
		return err
	}

	repo, err := r.repository(parsed, opts)
	if err != nil {
		return err
	}

	if err := pushTo(ctx, string(dir), repo, parsed.Reference, opts.OCICompat); err != nil {
		return compatHint(fmt.Errorf("push to %q: %w", ref, err))
	}

	return nil
}

// Pull fetches the artifact at ref and extracts its reports into dir.
func (r *ORASRegistry) Pull(ctx context.Context, ref string, dir m.Path, opts domain.RegistryOptions) (string, error) {
	parsed, err := parseReference(ref)
	if err != nil {
		return "", err
	}

	repo, err := r.repository(parsed, opts)
	if err != nil {
		return "", err
	}

	digest, err := pullFrom(ctx, repo, parsed.Reference, string(dir))
	if err != nil {
		return "", fmt.Errorf("pull from %q: %w", ref, err)
	}

	return digest, nil
}

// pushTo packages dir and copies it to dst under tag.
func pushTo(ctx context.Context, dir string, dst oras.Target, tag string, compat bool) error {
	staging, err := os.MkdirTemp("", "gooze-oci-push-*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}

	defer func() { _ = os.RemoveAll(staging) }()

	archivePath := filepath.Join(staging, reportsBlobName)
	if err := tarGz(dir, archivePath); err != nil {
		return fmt.Errorf("package reports: %w", err)
	}

	store, err := file.New(staging)
	if err != nil {
		return fmt.Errorf("create file store: %w", err)
	}

	defer func() { _ = store.Close() }()

	if _, err := (&ORASRegistry{}).pack(ctx, store, archivePath, tag, compat); err != nil {
		return err
	}

	if _, err := oras.Copy(ctx, store, tag, dst, tag, oras.DefaultCopyOptions); err != nil {
		return err
	}

	return nil
}

// pullFrom copies the artifact at reference from src, replaces the stored
// reports in dir with its reports layer and returns the manifest digest. Both
// packagings are read: gooze's media types and compat.
func pullFrom(ctx context.Context, src oras.ReadOnlyTarget, reference, dir string) (string, error) {
	staging, err := os.MkdirTemp("", "gooze-oci-pull-*")
	if err != nil {
		return "", fmt.Errorf("create staging dir: %w", err)
	}

	defer func() { _ = os.RemoveAll(staging) }()

	store, err := file.New(filepath.Join(staging, "store"))
	if err != nil {
		return "", fmt.Errorf("create file store: %w", err)
	}

	defer func() { _ = store.Close() }()

	root, err := oras.Copy(ctx, src, reference, store, reference, oras.DefaultCopyOptions)
	if err != nil {
		return "", err
	}

	raw, err := content.FetchAll(ctx, store, root)
	if err != nil {
		return "", fmt.Errorf("read manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", fmt.Errorf("read manifest: %w", err)
	}

	layer, ok := reportsLayer(manifest.Layers)
	if !ok {
		return "", fmt.Errorf("not a gooze reports artifact (no %s, %s or %s layer)", reportsBlobMediaType, compatBlobMediaType, reportsVersionAnnotation)
	}

	archivePath := filepath.Join(staging, reportsBlobName)
	if err := saveBlob(ctx, store, layer, archivePath); err != nil {
		return "", err
	}

	if err := removeStoredReports(dir); err != nil {
		return "", err
	}

	if err := unTarGz(archivePath, dir); err != nil {
		return "", fmt.Errorf("unpack reports: %w", err)
	}

	return root.Digest.String(), nil
}

// removeStoredReports empties dir but for the shard dirs and the narrowed
// run's dir, which wait for gooze report merge. A missing dir is empty.
func removeStoredReports(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("read reports dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() && (entry.Name() == domain.RunDirName || strings.HasPrefix(entry.Name(), domain.ShardDirPrefix)) {
			continue
		}

		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return fmt.Errorf("remove stored reports: %w", err)
		}
	}

	return nil
}

// reportsLayer returns the layer holding the reports: the one annotated
// dev.gooze.reports.version, else the first with gooze's or compat's media type.
func reportsLayer(layers []ocispec.Descriptor) (ocispec.Descriptor, bool) {
	for _, layer := range layers {
		if _, ok := layer.Annotations[reportsVersionAnnotation]; ok {
			return layer, true
		}
	}

	for _, layer := range layers {
		if layer.MediaType == reportsBlobMediaType || layer.MediaType == compatBlobMediaType {
			return layer, true
		}
	}

	return ocispec.Descriptor{}, false
}

// saveBlob writes the verified content of desc from store to path.
func saveBlob(ctx context.Context, store content.Fetcher, desc ocispec.Descriptor, path string) error {
	rc, err := store.Fetch(ctx, desc)
	if err != nil {
		return fmt.Errorf("read reports layer: %w", err)
	}

	defer func() { _ = rc.Close() }()

	out, err := os.Create(path) //nolint:gosec // path is an internal staging path.
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}

	if _, err := io.Copy(out, content.NewVerifyReader(rc, desc)); err != nil {
		return errors.Join(fmt.Errorf("read reports layer: %w", err), out.Close())
	}

	return out.Close()
}

// compatHint adds how to fix it to a push a registry rejected for an unknown
// media type (GitLab's answers 400 "manifest invalid: unknown media type").
func compatHint(err error) error {
	var resp *errcode.ErrorResponse
	if !errors.As(err, &resp) || resp.StatusCode != http.StatusBadRequest || !strings.Contains(err.Error(), "unknown media type") {
		return err
	}

	return fmt.Errorf("%w\nthe registry accepts only allowlisted media types: set report.oci.compat=true "+
		"(GOOZE_REPORT_OCI_COMPAT=true or --oci-compat) to push with compat media types; see %s", err, supportedMediaTypesDoc)
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

// pack adds the archive as a layer and tags a manifest wrapping it. compat
// packs with only media types GitLab's registry accepts: an image manifest
// with an unknown config and no artifactType, and an application/tar+gzip
// layer annotated dev.gooze.reports.version. Otherwise the manifest has the
// gooze artifactType and an application/gzip layer.
func (r *ORASRegistry) pack(ctx context.Context, store *file.Store, archivePath, tag string, compat bool) (ocispec.Descriptor, error) {
	mediaType, version, artifactType := reportsBlobMediaType, oras.PackManifestVersion1_1, reportsArtifactType
	if compat {
		mediaType, version, artifactType = compatBlobMediaType, oras.PackManifestVersion1_0, ""
	}

	layer, err := store.Add(ctx, reportsBlobName, mediaType, archivePath)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("add reports blob: %w", err)
	}

	if compat {
		layer.Annotations[reportsVersionAnnotation] = reportsVersion
	}

	manifest, err := oras.PackManifest(ctx, store, version, artifactType, oras.PackManifestOptions{
		Layers: []ocispec.Descriptor{layer},
	})
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pack manifest: %w", err)
	}

	if err := store.Tag(ctx, manifest, tag); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("tag manifest: %w", err)
	}

	return manifest, nil
}

// repository builds an authenticated remote repository client for ref.
func (r *ORASRegistry) repository(ref registry.Reference, opts domain.RegistryOptions) (*remote.Repository, error) {
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
func registryClient(host string, opts domain.RegistryOptions) (*auth.Client, error) {
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

	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12} //nolint:gosec // --insecure is an explicit opt-in.

	return &http.Client{Transport: retry.NewTransport(transport)}
}
