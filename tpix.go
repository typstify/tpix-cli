package tpix

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/typstify/tpix-cli/api"
	"github.com/typstify/tpix-cli/bundler"
	"github.com/typstify/tpix-cli/deps"
	"github.com/typstify/tpix-cli/storage"
)

type ReportFunc func(message string)

// ZoteroLibrary represents a Zotero library.
type ZoteroLibrary = api.ZoteroLibrary

type TpixSdk struct {
	store  storage.PackageStore
	client *api.ApiClient
	// output reporter
	reporter ReportFunc
}

func NewTpixSdk(httpClient *api.HttpClient, store storage.PackageStore) *TpixSdk {
	return &TpixSdk{
		store:  store,
		client: api.NewApiClient(httpClient, store),
	}
}

func (t *TpixSdk) WithReporter(reporter ReportFunc) {
	t.reporter = reporter
}

// ResolvedPackage is a package that was processed during a fetch operation,
// together with whether it was already present in the local cache.
type ResolvedPackage struct {
	Package deps.Dependency
	Cached  bool
}

// DirectDependency is a direct import discovered while scanning a project.
type DirectDependency struct {
	Package deps.Dependency
	Cached  bool
}

// PullResult is the outcome of DownloadProjectDependencies.
type PullResult struct {
	ProjectDir string
	DryRun     bool
	Direct     []DirectDependency
	Packages   []ResolvedPackage
}

// fetchWithDeps downloads a package and its transitive dependencies.
// visited tracks already-processed packages to prevent infinite loops, while
// resolved records every processed package together with its cache status.
func (t *TpixSdk) fetchWithDeps(pkg deps.Dependency, visited *[]deps.Dependency, resolved *[]ResolvedPackage, noDeps bool) error {
	if slices.Contains(*visited, pkg) {
		return nil
	}

	*visited = append(*visited, pkg)

	isCached, err := t.store.Has(pkg)
	if err != nil {
		return err
	}

	*resolved = append(*resolved, ResolvedPackage{Package: pkg, Cached: isCached})

	if isCached {
		if t.reporter != nil {
			t.reporter(fmt.Sprintf("  Already cached: %s\n", pkg))
		}
		// Do not return early, check if dependencies are satisfied.
	} else {
		if t.reporter != nil {
			t.reporter(fmt.Sprintf("  Downloading %s...\n", pkg))
		}
		if err := t.client.DownloadPackage(pkg.Namespace, pkg.Name, pkg.Version); err != nil {
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
		if err := t.fetchWithDeps(depSpec, visited, resolved, false); err != nil {
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

// DownloadPackage download Typst packages from TPIX server and returns the
// resolved package list, each tagged with whether it was already cached.
//
// pkgSpec should follow the pattern:  @namespace/name:version. Refer to [deps.ParseDependency] to know details.
// If noDeps is true, it will skip fetching transitive dependencies.
//
// The first item of the returned slice will always be the pkgSpec.
func (t *TpixSdk) DownloadPackage(pkgSpec string, noDeps bool) ([]ResolvedPackage, error) {
	// Parse namespace/name:version
	spec := deps.ParseDependency(pkgSpec)

	if !spec.IsValid() {
		return nil, errors.New("invalid package spec, use format @namespace/name:version")
	}

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

	if t.store == nil {
		return nil, fmt.Errorf("typst cache store not configured")
	}

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Resolving %s...\n", spec))
	}

	var visited []deps.Dependency
	var resolved []ResolvedPackage
	if err := t.fetchWithDeps(spec, &visited, &resolved, noDeps); err != nil {
		return nil, err
	}

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Done. %d package(s) resolved.\n", len(resolved)))
	}

	return resolved, nil
}

func (t *TpixSdk) DownloadProjectDependencies(projectDir string, dryRun bool) (*PullResult, error) {
	// Scan project directory for .typ imports
	if projectDir == "" {
		return nil, fmt.Errorf("invalid working directory: %s", projectDir)
	}

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Scanning %s for package imports...\n", projectDir))
	}

	discovered, err := deps.ExtractFromDirectory(projectDir)
	if err != nil {
		return nil, fmt.Errorf("failed to scan for imports: %w", err)
	}

	result := &PullResult{ProjectDir: projectDir, DryRun: dryRun}

	// Resolve the cache status of every direct import at scan time.
	for _, dep := range discovered {
		depSpec := deps.Dependency{
			Namespace: dep.Namespace,
			Name:      dep.Name,
			Version:   dep.Version,
		}

		cached, err := t.store.Has(depSpec)
		if err != nil {
			return nil, err
		}
		result.Direct = append(result.Direct, DirectDependency{Package: depSpec, Cached: cached})
	}

	if len(discovered) == 0 {
		if t.reporter != nil {
			t.reporter(fmt.Sprintln("No package imports found."))
		}
		return result, nil
	}

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Found %d direct dependency(ies).\n", len(discovered)))
	}

	if dryRun {
		for _, d := range result.Direct {
			status := "missing"
			if d.Cached {
				status = "cached"
			}

			if t.reporter != nil {
				t.reporter(fmt.Sprintf("  %s [%s]\n", d.Package, status))
			}
		}
		return result, nil
	}

	var visited []deps.Dependency
	var resolved []ResolvedPackage
	for _, dep := range discovered {
		depSpec := deps.Dependency{
			Namespace: dep.Namespace,
			Name:      dep.Name,
			Version:   dep.Version,
		}
		if err := t.fetchWithDeps(depSpec, &visited, &resolved, false); err != nil {
			return nil, err
		}
	}
	result.Packages = resolved

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Done. %d package(s) resolved.\n", len(resolved)))
	}

	return result, nil
}

// DependencyNode is a package together with its resolved dependencies.
type DependencyNode struct {
	Package  deps.Dependency
	Cached   bool
	Children []*DependencyNode
}

// DependencyGraph is the resolved dependency graph of a package.
type DependencyGraph struct {
	Root *DependencyNode
	// Packages is the flattened, de-duplicated set of all packages in the graph.
	Packages []ResolvedPackage
}

// ResolveDependencies resolves the full dependency graph of pkgSpec without
// downloading any package. Cache status is read from the local store.
func (t *TpixSdk) ResolveDependencies(pkgSpec string) (*DependencyGraph, error) {
	spec := deps.ParseDependency(pkgSpec)
	if !spec.IsValid() {
		return nil, errors.New("invalid package spec, use format @namespace/name:version")
	}

	if spec.Version == "" {
		pkg, err := t.client.FetchPackage(spec.Namespace, spec.Name)
		if err != nil {
			return nil, err
		}
		if len(pkg.Versions) == 0 {
			return nil, fmt.Errorf("no versions available for package")
		}
		spec.Version = pkg.Versions[0].Version
	}

	seen := make(map[deps.Dependency]bool)
	var flat []ResolvedPackage

	cachedStatus := func(pkg deps.Dependency) bool {
		if t.store == nil {
			return false
		}
		cached, err := t.store.Has(pkg)
		return err == nil && cached
	}

	var walk func(pkg deps.Dependency) *DependencyNode
	walk = func(pkg deps.Dependency) *DependencyNode {
		node := &DependencyNode{Package: pkg, Cached: cachedStatus(pkg)}

		// A package already seen elsewhere in the graph becomes a leaf, which
		// both de-duplicates shared dependencies and prevents infinite cycles.
		if seen[pkg] {
			return node
		}
		seen[pkg] = true
		flat = append(flat, ResolvedPackage{Package: pkg, Cached: node.Cached})

		depInfos, err := t.client.FetchDependencies(pkg.Namespace, pkg.Name, pkg.Version)
		if err != nil {
			// Non-fatal: the server may not have dependency data for older packages.
			return node
		}

		for _, dep := range depInfos {
			node.Children = append(node.Children, walk(deps.Dependency{
				Namespace: dep.Namespace,
				Name:      dep.Name,
				Version:   dep.Version,
			}))
		}
		return node
	}

	root := walk(spec)
	return &DependencyGraph{Root: root, Packages: flat}, nil
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

func (t *TpixSdk) PushPackage(packagePath string, namespace string) (*api.UploadResponse, error) {
	// Check if file exists
	info, err := os.Stat(packagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to access package: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory, not a package file", packagePath)
	}

	if t.reporter != nil {
		t.reporter(fmt.Sprintf("Uploading %s to namespace %s...\n", packagePath, namespace))
	}

	resp, err := t.client.UploadPackage(packagePath, namespace)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
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

	return resp, nil
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
