package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/typstify/tpix-cli/api"
	"github.com/typstify/tpix-cli/bundler"
	"github.com/typstify/tpix-cli/deps"
	"github.com/typstify/tpix-cli/utils"
)

const (
	pollInterval = 5 * time.Second
)

type ReportFunc func(message string)

// ZoteroLibrary represents a Zotero library.
type ZoteroLibrary = api.ZoteroLibrary

// Helper functions

// ParsePkgSpec parses a package spec in the format @namespace/name:version
// Returns namespace, name, and version (version may be empty)
func ParsePkgSpec(pkgSpec string) (namespace, name, version string) {
	// Remove leading @ and split on /
	s := strings.TrimPrefix(pkgSpec, "@")
	parts := strings.SplitN(s, "/", 2)
	if len(parts) < 2 {
		return
	}
	namespace = parts[0]

	// Split name and version on :
	nameVer := strings.SplitN(parts[1], ":", 2)
	name = nameVer[0]
	if len(nameVer) > 1 {
		version = nameVer[1]
	}
	return
}

// isPackageCached checks if a package version is already in the local cache.
func isPackageCached(cacheDir, namespace, name, version string) bool {
	pkgDir := filepath.Join(cacheDir, namespace, name, version)
	info, err := os.Stat(pkgDir)
	return err == nil && info.IsDir()
}

// fetchWithDeps downloads a package and its transitive dependencies.
// visited tracks already-processed packages to prevent infinite loops.
func fetchWithDeps(namespace, name, version, cacheDir string, visited map[string]bool, noDeps bool, reporter ReportFunc) error {
	key := fmt.Sprintf("@%s/%s:%s", namespace, name, version)
	if visited[key] {
		return nil
	}
	visited[key] = true

	if isPackageCached(cacheDir, namespace, name, version) {
		if reporter != nil {
			reporter(fmt.Sprintf("  Already cached: %s\n", key))
		}
		// Do not return early, check if dependencies are satisfied.
	} else {
		if reporter != nil {
			reporter(fmt.Sprintf("  Downloading %s...\n", key))
		}
		if err := api.DownloadPackage(namespace, name, version, cacheDir); err != nil {
			return fmt.Errorf("failed to download %s: %w", key, err)
		}
	}

	if noDeps {
		return nil
	}

	// Fetch and resolve transitive dependencies
	depInfos, err := api.FetchDependencies(namespace, name, version)
	if err != nil {
		// Non-fatal: the server may not have dependency data for older packages
		return nil
	}

	for _, dep := range depInfos {
		if err := fetchWithDeps(dep.Namespace, dep.Name, dep.Version, cacheDir, visited, false, reporter); err != nil {
			return err
		}
	}

	return nil
}

func StartLogin() (*api.DeviceCodeResponse, error) {
	deviceResp, err := api.StartDeviceLogin()
	if err != nil {
		return nil, err
	}

	verifyUrl := deviceResp.VerificationURI + "?user_code=" + deviceResp.UserCode
	// open the url for user
	utils.OpenURL(verifyUrl)

	return deviceResp, nil
}

func PollLoginResult(deviceCode string, expiresIn int, reporter ReportFunc) (*api.TokenResponse, error) {
	timeout := time.After(time.Duration(expiresIn) * time.Second)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	hostname, _ := os.Hostname()

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("device code expired, please try again.")
		case <-ticker.C:
			tokenResp, pending, err := api.PollForToken(deviceCode, hostname)
			if err != nil {
				return nil, err
			}
			if !pending {
				return tokenResp, nil
			}

			if reporter != nil {
				fmt.Print(".")
			}
		}
	}
}

// SearchPackages searches Typst packages from TPIX server.
func SearchPackages(namespace string, query string, kind string, category string, sort string, limit int) (*api.SearchResponse, error) {
	if query == "" {
		return nil, errors.New("missing query")
	}

	if limit <= 0 {
		limit = 20
	}

	return api.SearchPackages(query, namespace, kind, category, sort, limit)
}

// DownloadPackage download Typst packages from TPIX server.
// pkgSpec should follow the pattern:  @namespace/name:version. Refer to [parsePkgSpec] to know details.
// If noDeps is true, it will skip fetching transitive dependencies.
func DownloadPackage(pkgSpec string, cacheDir string, noDeps bool, reporter ReportFunc) (int, error) {
	// Parse namespace/name:version
	namespace, name, version := ParsePkgSpec(pkgSpec)

	if version == "" {
		// Get latest version first
		pkg, err := api.FetchPackage(namespace, name)
		if err != nil {
			return 0, err
		}
		if len(pkg.Versions) == 0 {
			return 0, fmt.Errorf("no versions available for package")
		}
		version = pkg.Versions[len(pkg.Versions)-1].Version
	}

	if cacheDir == "" {
		return 0, fmt.Errorf("typst cache directory not configured")
	}

	if reporter != nil {
		reporter(fmt.Sprintf("Resolving @%s/%s:%s...\n", namespace, name, version))
	}

	visited := make(map[string]bool)
	if err := fetchWithDeps(namespace, name, version, cacheDir, visited, noDeps, reporter); err != nil {
		return 0, err
	}

	if reporter != nil {
		reporter(fmt.Sprintf("Done. %d package(s) resolved.\n", visited))
	}

	return len(visited), nil
}

func DownloadProjectDependencies(projectDir string, cacheDir string, dryRun bool, reporter ReportFunc) error {
	// Scan project directory for .typ imports
	if projectDir == "" {
		return fmt.Errorf("invalid working directory: %s", projectDir)
	}

	if reporter != nil {
		reporter(fmt.Sprintf("Scanning %s for package imports...\n", projectDir))
	}

	discovered, err := deps.ExtractFromDirectory(projectDir)
	if err != nil {
		return fmt.Errorf("failed to scan for imports: %w", err)
	}

	if len(discovered) == 0 {
		if reporter != nil {
			reporter(fmt.Sprintln("No package imports found."))
		}
		return nil
	}

	if reporter != nil {
		reporter(fmt.Sprintf("Found %d direct dependency(ies).\n", len(discovered)))
	}

	if dryRun {
		for _, dep := range discovered {
			cached := isPackageCached(cacheDir, dep.Namespace, dep.Name, dep.Version)
			status := "missing"
			if cached {
				status = "cached"
			}

			if reporter != nil {
				reporter(fmt.Sprintf("  %s [%s]\n", dep.Key(), status))
			}
		}
		return nil
	}

	visited := make(map[string]bool)
	for _, dep := range discovered {
		if err := fetchWithDeps(dep.Namespace, dep.Name, dep.Version, cacheDir, visited, false, reporter); err != nil {
			return err
		}
	}

	if reporter != nil {
		reporter(fmt.Sprintf("Done. %d package(s) resolved.\n", len(visited)))
	}

	return nil
}

func QueryPackage(pkgSpec string) (*api.PackageResponse, error) {
	// Parse namespace/name
	namespace, name, _ := ParsePkgSpec(pkgSpec)

	pkg, err := api.FetchPackage(namespace, name)
	if err != nil {
		return nil, err
	}

	return pkg, nil

}

func BundlePackage(srcDir string, outputFile string, excludedFiles []string) (string, error) {

	// Check if directory exists
	info, err := os.Stat(srcDir)
	if err != nil {
		return "", fmt.Errorf("failed to access directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", srcDir)
	}

	// Check for typst.toml
	manifestPath := filepath.Join(srcDir, "typst.toml")
	if _, err := os.Stat(manifestPath); err != nil {
		return "", fmt.Errorf("typst.toml not found in %s - a valid manifest is required", srcDir)
	}

	// Determine output path
	if outputFile == "" {
		// Use directory name with .tar.gz extension
		outputFile = filepath.Base(srcDir) + ".tar.gz"
	}

	// Create package
	creator := bundler.NewPackageCreator(excludedFiles)
	if err := creator.CreatePackage(srcDir, outputFile); err != nil {
		return "", fmt.Errorf("failed to create package: %w", err)
	}

	return outputFile, nil
}

func PushPackage(packagePath string, namespace string, reporter ReportFunc) error {
	// Check if file exists
	info, err := os.Stat(packagePath)
	if err != nil {
		return fmt.Errorf("failed to access package: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, not a package file", packagePath)
	}

	if reporter != nil {
		reporter(fmt.Sprintf("Uploading %s to namespace %s...\n", packagePath, namespace))
	}

	resp, err := api.UploadPackage(packagePath, namespace)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	if resp.SHA256 != "" {
		if reporter != nil {
			reporter(fmt.Sprintf("Successfully uploaded package: @%s/%s:%s\n", namespace, resp.Package, resp.Version))
		}
	} else {
		if reporter != nil {
			reporter("Upload failed, report: \n")

			for _, r := range resp.ValidateReport {
				reporter(fmt.Sprintf("\t%s\n", r))
			}
		}
	}

	return nil
}

// ListZoteroLibraries returns the list of Zotero libraries accessible to the user.
func ListZoteroLibraries() ([]api.ZoteroLibrary, error) {
	return api.QueryZoteroLibraries()
}

// CreateZoteroExport creates an export target on the TPIX server.
func CreateZoteroExport(name string, namespaceID string, libraryType string, libraryID int64, collectionKey string, format string, reporter ReportFunc) (string, error) {
	target := api.ZoteroExportTarget{
		NamespaceID:   namespaceID,
		Name:          name,
		LibraryType:   libraryType,
		LibraryID:     libraryID,
		CollectionKey: collectionKey,
		Format:        format,
	}

	if reporter != nil {
		reporter(fmt.Sprintf("Creating export for library %s:%d, collection %s...\n", libraryType, libraryID, collectionKey))
	}

	exportID, err := api.CreateZoteroExport(target)
	if err != nil {
		return "", fmt.Errorf("failed to create export: %w", err)
	}

	if reporter != nil {
		reporter(fmt.Sprintf("Export created: %s\n", exportID))
	}

	return exportID, nil
}

// FetchZoteroExport fetches the content of a Zotero export.
func FetchZoteroExport(exportID string, writer io.Writer) error {
	return api.FetchLatestZoteroCollections(exportID, writer)
}

// DeleteZoteroExport deletes an existing Zotero export.
func DeleteZoteroExport(exportID string, reporter ReportFunc) error {
	if reporter != nil {
		reporter(fmt.Sprintf("Deleting export %s...\n", exportID))
	}

	if err := api.DeleteZoteroExport(exportID); err != nil {
		return fmt.Errorf("failed to delete export: %w", err)
	}

	if reporter != nil {
		reporter("Export deleted.\n")
	}

	return nil
}
