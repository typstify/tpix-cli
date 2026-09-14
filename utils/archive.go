package utils

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ExtractTarGz extracts a tar.gz archive to the specified directory.
func ExtractTarGz(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	return ExtractTarGzFromReader(file, destDir)
}

// ExtractTarGzFromReader reads from the archiveReader and extracts its content
// to destDir.
//
// Entries are written through an os.Root, so an entry whose name escapes
// destDir (e.g. "../../etc/passwd") is rejected instead of being written
// outside the destination (Zip Slip).
func ExtractTarGzFromReader(archiveReader io.Reader, destDir string) error {
	gzr, err := gzip.NewReader(archiveReader)
	if err != nil {
		return err
	}
	defer gzr.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	root, err := os.OpenRoot(destDir)
	if err != nil {
		return err
	}
	defer root.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		name, err := cleanArchiveName(header.Name)
		if err != nil {
			return err
		}
		if name == "" {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o755); err != nil {
				return fmt.Errorf("failed to create directory %q: %w", header.Name, err)
			}
		case tar.TypeReg:
			if err := writeEntry(root, name, tr); err != nil {
				return fmt.Errorf("failed to write %q: %w", header.Name, err)
			}
		}
	}

	return nil
}

// writeEntry writes r to name inside root, creating parent directories. All
// paths go through root, so entries cannot escape destDir.
func writeEntry(root *os.Root, name string, r io.Reader) error {
	if dir := filepath.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}

	return f.Close()
}

// cleanArchiveName normalizes a tar entry name into an OS path that is safe to
// pass to os.Root. It rejects absolute paths and returns an empty string for
// entries that refer to the destination root itself. Escaping entries such as
// "../../etc/passwd" are left intact here and rejected by os.Root when used.
func cleanArchiveName(name string) (string, error) {
	if name == "" {
		return "", nil
	}

	name = filepath.Clean(filepath.FromSlash(name))
	if name == "." || name == string(filepath.Separator) {
		return "", nil
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("archive entry %q has an absolute path", name)
	}

	return name, nil
}
