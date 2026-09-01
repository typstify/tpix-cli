package tpix

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/typstify/tpix-cli/api"
	"github.com/typstify/tpix-cli/bundler"
	"github.com/typstify/tpix-cli/deps"
)

type ReportFunc func(message string)

// ZoteroLibrary represents a Zotero library.
type ZoteroLibrary = api.ZoteroLibrary

type TpixSdk struct {
	client *api.ApiClient
	// output reporter
	reporter ReportFunc
}

func NewTpixSdk(httpClient *api.HttpClient) *TpixSdk {
	return &TpixSdk{
		client: api.NewApiClient(httpClient),
	}
}

// isPackageCached checks if a package version is already in the local cache.
func isPackageCached(cacheDir string, pkg deps.Dependency) bool {
	if !pkg.Partial() {
		return false
	}

	pkgDir := filepath.Join(cacheDir, pkg.Namespace, pkg.Name, pkg.Version)
	info, err := os.Stat(pkgDir)
	return err == nil && info.IsDir()
}

func (t *TpixSdk) WithReporter(reporter ReportFunc) {
	t.reporter = reporter
}

// fetchWithDeps downloads a package and its transitive dependencies.
// visited tracks already-processed packages to prevent infinite loops.
func (t *TpixSdk) fetchWithDeps(pkg deps.Dependency, cacheDir string, visited *[]deps.Dependency, noDeps bool) error {
	if slices.Contains(*visited, pkg) {
		return nil
	}

	*visited = append(*visited, pkg)

	if isPackageCached(cacheDir, pkg) {
		if t.reporter != nil {
			t.reporter(fmt.Sprintf("  Already cached: %s\n", pkg))
		}
		// Do not return early, check if dependencies are satisfied.
	} else {
		if t.reporter != nil {
			t.reporter(fmt.Sprintf("  Downloading %s...\n", pkg))
		}
		if err := t.client.DownloadPackage(pkg.Namespace, pkg.Name, pkg.Version, cacheDir); err != nil {
			return fmt.Errorf("failed to download %s: %w", pkg, err)
		}
	}

	if noDeps {
		return nil
	}

	// Fetch and resolve transitive dependencies
	depInfos, err := t.client.FetchDependencies(pkg.Namespace, pkg.Name, pkg.Version)
	if err != nil {
		// Non-fatal: the server may not have dependency data for older packages
		return nil
	}

	for _, dep := range depInfos {
		depSpec := deps.Dependency{
			Namespace: dep.Namespace,
			Name:      dep.Name,
			Version:   dep.Version,
		}
		if err := t.fetchWithDeps(depSpec, cacheDir, visited, false); err != nil {
			return err
		}
	}

	return nil
}

// SearchPackages searches Typst packages from TPIX server.
func (t *TpixSdk) SearchPackages(namespace string, query string, kind string, category string, sort string, limit int) (*api.SearchResponse, error) {
	if limit <= 0 {
		limit = 20
	}

	return t.client.SearchPackages(query, namespace, kind, category, sort, limit)
}

// DownloadPackage download Typst packages from TPIX server and returns downloaded package spec list.
//
// pkgSpec should follow the pattern:  @namespace/name:version. Refer to [deps.ParseDependency] to know details.
// If noDeps is true, it will skip fetching transitive dependencies.
//
// The first item of returned slice will always be the pkgSpec.
func (t *TpixSdk) DownloadPackage(pkgSpec string, cacheDir string, noDeps bool) ([]deps.Dependency, error) {
	// Parse namespace/name:version
	spec := deps.ParseDependency(pkgSpec)

	if spec.Version == "" {
		// Get latest version first
		pkg, err := t.client.FetchPackage(spec.Namespace, spec.Name)
		if err != nil {
			return nil, err
		}
		if len(pkg.Versions) == 0 {
			return nil, fmt.Errorf("no versions available for package")
		}
		spec.Version = pkg.Versions[0].Version
	}

	if cacheDir == "" {
		return nil, fmt.Errorf("typst cache directory not configured")
	}

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Resolving %s...\n", spec))
	}

	var visited []deps.Dependency
	if err := t.fetchWithDeps(spec, cacheDir, &visited, noDeps); err != nil {
		return nil, err
	}

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Done. %d package(s) resolved.\n", len(visited)))
	}

	log.Println("pkgPaths: ", visited)
	return visited, nil
}

func (t *TpixSdk) DownloadProjectDependencies(projectDir string, cacheDir string, dryRun bool) error {
	// Scan project directory for .typ imports
	if projectDir == "" {
		return fmt.Errorf("invalid working directory: %s", projectDir)
	}

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Scanning %s for package imports...\n", projectDir))
	}

	discovered, err := deps.ExtractFromDirectory(projectDir)
	if err != nil {
		return fmt.Errorf("failed to scan for imports: %w", err)
	}

	if len(discovered) == 0 {
		if t.reporter != nil {
			t.reporter(fmt.Sprintln("No package imports found."))
		}
		return nil
	}

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Found %d direct dependency(ies).\n", len(discovered)))
	}

	if dryRun {
		for _, dep := range discovered {
			depSpec := deps.Dependency{
				Namespace: dep.Namespace,
				Name:      dep.Name,
				Version:   dep.Version,
			}
			cached := isPackageCached(cacheDir, depSpec)
			status := "missing"
			if cached {
				status = "cached"
			}

			if t.reporter != nil {
				t.reporter(fmt.Sprintf("  %s [%s]\n", dep.String(), status))
			}
		}
		return nil
	}

	visited := make([]deps.Dependency, 0)
	for _, dep := range discovered {
		depSpec := deps.Dependency{
			Namespace: dep.Namespace,
			Name:      dep.Name,
			Version:   dep.Version,
		}
		if err := t.fetchWithDeps(depSpec, cacheDir, &visited, false); err != nil {
			return err
		}
	}

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Done. %d package(s) resolved.\n", len(visited)))
	}

	return nil
}

func (t *TpixSdk) QueryPackage(pkgSpec string) (*api.PackageResponse, error) {
	// Parse namespace/name
	spec := deps.ParseDependency(pkgSpec)

	pkg, err := t.client.FetchPackage(spec.Namespace, spec.Name)
	if err != nil {
		return nil, err
	}

	return pkg, nil

}

func (t *TpixSdk) BundlePackage(srcDir string, outputFile string, excludedFiles []string) (string, error) {
	srcDir, err := filepath.Abs(srcDir)
	if err != nil {
		return "", err
	}

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
		outputFile = filepath.Join(srcDir, filepath.Base(srcDir)+".tar.gz")
	}

	// Create package
	creator := bundler.NewPackageCreator(excludedFiles)
	if err := creator.CreatePackage(srcDir, outputFile); err != nil {
		return "", fmt.Errorf("failed to create package: %w", err)
	}

	return outputFile, nil
}

func (t *TpixSdk) PushPackage(packagePath string, namespace string) error {
	// Check if file exists
	info, err := os.Stat(packagePath)
	if err != nil {
		return fmt.Errorf("failed to access package: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, not a package file", packagePath)
	}

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Uploading %s to namespace %s...\n", packagePath, namespace))
	}

	resp, err := t.client.UploadPackage(packagePath, namespace)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	if resp.SHA256 != "" {
		if t.reporter != nil {
			t.reporter(fmt.Sprintf("Successfully uploaded package: @%s/%s:%s\n", namespace, resp.Package, resp.Version))
		}
	} else {
		if t.reporter != nil {
			t.reporter("Upload failed, report: \n")

			for _, r := range resp.ValidateReport {
				t.reporter(fmt.Sprintf("\t%s\n", r))
			}
		}
	}

	return nil
}

// GetUserProfile queries the user profile from TPIX server.
func (t *TpixSdk) GetUserProfile() (*api.UserProfile, error) {
	return t.client.GetUserProfile()
}

// ListZoteroLibraries returns the list of Zotero libraries accessible to the user.
func (t *TpixSdk) ListZoteroLibraries() ([]api.ZoteroLibrary, error) {
	return t.client.QueryZoteroLibraries()
}

// CreateZoteroExport creates an export target on the TPIX server.
func (t *TpixSdk) CreateZoteroExport(name string, namespaceID string, libraryType string, libraryID int64, collectionKey string, format string) (string, error) {
	target := api.ZoteroExportTarget{
		NamespaceID:   namespaceID,
		Name:          name,
		LibraryType:   libraryType,
		LibraryID:     libraryID,
		CollectionKey: collectionKey,
		Format:        format,
	}

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Creating export for library %s:%d, collection %s...\n", libraryType, libraryID, collectionKey))
	}

	exportID, err := t.client.CreateZoteroExport(target)
	if err != nil {
		return "", fmt.Errorf("failed to create export: %w", err)
	}

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Export created: %s\n", exportID))
	}

	return exportID, nil
}

// FetchZoteroExport fetches the content of a Zotero export.
func (t *TpixSdk) FetchZoteroExport(exportID string, writer io.Writer) error {
	return t.client.FetchLatestZoteroCollections(exportID, writer)
}

// DeleteZoteroExport deletes an existing Zotero export.
func (t *TpixSdk) DeleteZoteroExport(exportID string) error {
	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Deleting export %s...\n", exportID))
	}

	if err := t.client.DeleteZoteroExport(exportID); err != nil {
		return fmt.Errorf("failed to delete export: %w", err)
	}

	if t.reporter != nil {
		t.reporter("Export deleted.\n")
	}

	return nil
}

// GetPackageIndex retrieves a markdown file containing all user
// accessible package/template metadata. This API is dedicated for LLM use.
func (t *TpixSdk) GetPackageIndex() (string, error) {
	return t.client.GetPackageIndex()
}

// GetNamespacePackages fetches all the packages in the specified namespace.
func (t *TpixSdk) GetNamespacePackages(namespace string) ([]api.PackageResponse, error) {
	return t.client.GetNamespacePackages(namespace)
}
