package bundler

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// PackageCreator creates a Typst package from a directory
type PackageCreator struct {
	exclude []string
}

// NewPackageCreator creates a new PackageCreator
func NewPackageCreator(exclude []string) *PackageCreator {
	return &PackageCreator{
		exclude: exclude,
	}
}

// CreatePackage creates a tar.gz package from the source directory
func (p *PackageCreator) CreatePackage(srcDir, outputPath string) error {
	// Read and validate manifest
	manifestPath := filepath.Join(srcDir, "typst.toml")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read typst.toml: %w", err)
	}

	var manifest Manifest
	if err := DecodeBytes(manifestData, &manifest); err != nil {
		return fmt.Errorf("failed to parse typst.toml: %w", err)
	}

	// Validate required fields
	if err := p.validateManifest(&manifest); err != nil {
		return err
	}

	// Merge exclude patterns from manifest
	excludePatterns := p.exclude
	if len(manifest.Package.Exclude) > 0 {
		excludePatterns = append(excludePatterns, manifest.Package.Exclude...)
	}

	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	gzw := gzip.NewWriter(outputFile)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	// Walk the source directory and add files to tar
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if path == outputPath {
			return nil
		}

		// Check if it starts with a dot
		if strings.HasPrefix(info.Name(), ".") {
			// Skip entire hidden directories
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Symlinks are not supported in packages
		if info.Mode()&os.ModeSymlink == os.ModeSymlink {
			return fmt.Errorf("symbol links are not supported: %s", path)
		}

		// Get relative path from source directory
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if relPath == "." {
			return nil
		}

		// Check if file should be excluded
		if p.shouldExclude(relPath, excludePatterns) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Use forward slashes for the archive
		header.Name = filepath.ToSlash(relPath)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// Write file content (skip directories)
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tw, file); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create package: %w", err)
	}

	return nil
}

// validateManifest validates that the manifest has required fields
func (p *PackageCreator) validateManifest(manifest *Manifest) error {
	if manifest.Package == nil {
		return fmt.Errorf("missing [package] section in typst.toml")
	}

	if manifest.Package.Name == "" {
		return fmt.Errorf("package name is required in typst.toml")
	}

	if manifest.Package.Version == "" {
		return fmt.Errorf("package version is required in typst.toml")
	}

	if manifest.Package.Entrypoint == "" {
		return fmt.Errorf("package entrypoint is required in typst.toml")
	}

	return nil
}

// shouldExclude checks if a path should be excluded based on patterns.
// This method tries to aligh how Typst package bundler handles the exclude fields,
// as shown here: https://github.com/typst/packages/blob/main/bundler/src/main.rs#L402
func (p *PackageCreator) shouldExclude(path string, patterns []string) bool {
	// Normalize path to use forward slashes
	path = filepath.ToSlash(path)

	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimPrefix(pattern, "./"))

		// Use doublestar for recursive (**) and standard glob matching.
		// This aligns with how
		matched, err := doublestar.Match(pattern, path)
		if err == nil && matched {
			return true
		}

		// Handle Directory Globs (e.g., "tests/" excluding all "tests/...")
		// If the pattern ends in / and the path starts with that pattern, exclude it.
		if strings.HasSuffix(pattern, "/") && strings.HasPrefix(path, pattern) {
			return true
		}
	}

	return false
}
