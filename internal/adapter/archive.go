package adapter

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// tarGz writes a gzip-compressed tar of srcDir's regular files and directories
// (paths relative to srcDir) to dest.
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

	if err := out.Close(); err != nil {
		return fmt.Errorf("close archive: %w", err)
	}

	return nil
}

func writeTarEntry(tw *tar.Writer, path, rel string, info os.FileInfo) error {
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil
	}

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

// unTarGz extracts a gzip-compressed tar at src into destDir, confining every
// entry to destDir via os.Root.
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

	root, err := os.OpenRoot(destDir)
	if err != nil {
		return fmt.Errorf("open dest dir: %w", err)
	}

	defer func() { _ = root.Close() }()

	tr := tar.NewReader(gz)

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		if err := extractTarEntry(tr, root, header); err != nil {
			return err
		}
	}
}

func extractTarEntry(tr *tar.Reader, root *os.Root, header *tar.Header) error {
	name, err := archiveEntryPath(header.Name)
	if err != nil {
		return err
	}

	switch header.Typeflag {
	case tar.TypeDir:
		return root.MkdirAll(name, dirPerm(header.Mode))
	case tar.TypeReg:
		return writeRegularFile(tr, root, name, header)
	default:
		// Skip symlinks and other unexpected entry types.
		return nil
	}
}

func writeRegularFile(tr *tar.Reader, root *os.Root, name string, header *tar.Header) error {
	if dir := filepath.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}

	if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("replace %q: %w", header.Name, err)
	}

	out, err := root.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, filePerm(header.Mode))
	if err != nil {
		return err
	}

	defer func() { _ = out.Close() }()

	if _, err := io.CopyN(out, tr, header.Size); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("write %q: %w", header.Name, err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("close %q: %w", header.Name, err)
	}

	return nil
}

func filePerm(mode int64) os.FileMode {
	return os.FileMode(mode).Perm() | 0o600 //nolint:gosec // mode comes from a tar header.
}

func dirPerm(mode int64) os.FileMode {
	return os.FileMode(mode).Perm() | 0o700 //nolint:gosec // mode comes from a tar header.
}

// archiveEntryPath normalises an archive entry name to a relative path inside
// the extraction root.
func archiveEntryPath(name string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(name))

	if cleaned == "." || cleaned == "" || filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" ||
		cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe path in archive: %q", name)
	}

	return cleaned, nil
}
