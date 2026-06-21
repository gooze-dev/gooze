package adapter

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// tarGz writes a gzip-compressed tar of srcDir's contents (paths relative to
// srcDir) to dest.
func tarGz(srcDir, dest string) error {
	out, err := os.Create(dest) //nolint:gosec // dest is an internal staging path.
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}

	defer func() { _ = out.Close() }()

	gz := gzip.NewWriter(out)

	defer func() { _ = gz.Close() }()

	tw := tar.NewWriter(gz)

	defer func() { _ = tw.Close() }()

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		if rel == "." {
			return nil
		}

		return writeTarEntry(tw, path, rel, info)
	})
	if err != nil {
		return err
	}

	// Flush writers before returning so dest is complete.
	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar: %w", err)
	}

	if err := gz.Close(); err != nil {
		return fmt.Errorf("close gzip: %w", err)
	}

	return nil
}

func writeTarEntry(tw *tar.Writer, path, rel string, info os.FileInfo) error {
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}

	header.Name = filepath.ToSlash(rel)

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write header %q: %w", rel, err)
	}

	if info.IsDir() {
		return nil
	}

	file, err := os.Open(path) //nolint:gosec // path comes from walking the controlled source dir.
	if err != nil {
		return err
	}

	defer func() { _ = file.Close() }()

	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("copy %q: %w", rel, err)
	}

	return nil
}

// unTarGz extracts a gzip-compressed tar at src into destDir.
func unTarGz(src, destDir string) error {
	in, err := os.Open(src) //nolint:gosec // src is an internal staging path.
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}

	defer func() { _ = in.Close() }()

	gz, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}

	defer func() { _ = gz.Close() }()

	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	tr := tar.NewReader(gz)

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		if err := extractTarEntry(tr, destDir, header); err != nil {
			return err
		}
	}
}

func extractTarEntry(tr *tar.Reader, destDir string, header *tar.Header) error {
	target, err := safeJoin(destDir, header.Name)
	if err != nil {
		return err
	}

	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, 0o750)
	case tar.TypeReg:
		return writeRegularFile(tr, target, header)
	default:
		// Skip symlinks and other unexpected entry types.
		return nil
	}
}

func writeRegularFile(tr *tar.Reader, target string, header *tar.Header) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}

	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode)&0o600) //nolint:gosec // target is validated by safeJoin.
	if err != nil {
		return err
	}

	defer func() { _ = out.Close() }()

	// Bound the copy to the declared size to avoid a decompression bomb.
	if _, err := io.CopyN(out, tr, header.Size); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("write %q: %w", header.Name, err)
	}

	return nil
}

// safeJoin joins destDir and name, rejecting paths that escape destDir.
func safeJoin(destDir, name string) (string, error) {
	target := filepath.Join(destDir, filepath.FromSlash(name))

	cleanDir := filepath.Clean(destDir) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDir) {
		return "", fmt.Errorf("unsafe path in archive: %q", name)
	}

	return target, nil
}
