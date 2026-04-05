package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/typstify/tpix-cli/utils"
)

var (
	client *HttpClient
)

func Init(provider CredentialsProvider) {
	client = NewHttpClient(provider)
}

// SearchPackages fetches packages matching a query from the TPIX server.
func SearchPackages(query, namespace string, limit int) (*SearchResponse, error) {
	url := fmt.Sprintf("/api/v1/search?q=%s", query)
	if namespace != "" {
		url += "&namespace=" + namespace
	}
	if limit > 0 {
		url += fmt.Sprintf("&limit=%d", limit)
	}

	resp, err := client.MakeRequest("GET", url, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to search packages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed: %s", string(body))
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// DownloadPackage downloads a package, extracts it to the cache directory,
// and optionally saves the archive to output path.
func DownloadPackage(namespace, name, version string, cacheDir string) error {
	url := fmt.Sprintf("/api/v1/download/%s/%s/%s", namespace, name, version)

	resp, err := client.MakeRequest("GET", url, nil, "")
	if err != nil {
		return fmt.Errorf("failed to download package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed: %s", string(body))
	}

	// Create temp file for the archive
	tmpFile, err := os.CreateTemp("", "tpix-*.tar.gz")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if cacheDir == "" {
		return fmt.Errorf("typst cache directory not set")
	}

	extractDir := filepath.Join(cacheDir, namespace, name, version)
	if err := utils.ExtractTarGz(tmpPath, extractDir); err != nil {
		return fmt.Errorf("failed to extract package: %w", err)
	}

	return nil
}

// FetchPackage fetches package details from the TPIX server.
func FetchPackage(namespace, name string) (*PackageResponse, error) {
	url := fmt.Sprintf("/api/v1/packages/%s/%s", namespace, name)
	resp, err := client.MakeRequest("GET", url, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get package: %s", string(body))
	}

	var pkg PackageResponse
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Fetch all versions
	versions, err := fetchPackageVersions(namespace, name)
	if err == nil && len(versions) > 0 {
		pkg.Versions = versions
	}

	return &pkg, nil
}

// FetchPackageVersions fetches all versions for a package.
func fetchPackageVersions(namespace, name string) ([]PackageVersionInfo, error) {
	url := fmt.Sprintf("/api/v1/packages/%s/%s/versions", namespace, name)
	resp, err := client.MakeRequest("GET", url, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get versions: %s", string(body))
	}

	var versionsResp PackageVersionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&versionsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return versionsResp.Versions, nil
}

// FetchDependencies fetches the dependencies for a specific package version.
func FetchDependencies(namespace, name, version string) ([]DependencyInfo, error) {
	url := fmt.Sprintf("/api/v1/packages/%s/%s/%s/dependencies", namespace, name, version)
	resp, err := client.MakeRequest("GET", url, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch dependencies: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get dependencies: %s", string(body))
	}

	var depsResp DependenciesResponse
	if err := json.NewDecoder(resp.Body).Decode(&depsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return depsResp.Dependencies, nil
}

// UploadPackage uploads a package to the TPIX server.
func UploadPackage(packagePath, namespace string) (*UploadResponse, error) {
	file, err := os.Open(packagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open package file: %w", err)
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file field
	part, err := writer.CreateFormFile("file", fileInfo.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	if err := writer.WriteField("namespace", namespace); err != nil {
		return nil, fmt.Errorf("failed to write namespace field: %w", err)
	}

	writer.Close()

	// Create request
	url := "/api/v1/packages/upload"
	resp, err := client.MakeRequest("POST", url, &buf, writer.FormDataContentType())
	if err != nil {
		return nil, fmt.Errorf("failed to upload package: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	var uploadResp UploadResponse
	if err := json.Unmarshal(body, &uploadResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &uploadResp, nil
}

// QueryZoteroLibraries fetches zotero libraries the current user has access
// permission. This includes libraries granted in personal account, and libraries
// granted by the namespace owners.
func QueryZoteroLibraries() ([]ZoteroLibrary, error) {
	url := "/api/v1/zotero/libraries"
	resp, err := client.MakeRequest("GET", url, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch zotero libraries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch zotero libraries: %s", string(body))
	}

	var libraries []ZoteroLibrary
	if err := json.NewDecoder(resp.Body).Decode(&libraries); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return libraries, nil
}

// CreateZoteroExport creates an export target on TPIX server.
// Requires a registered Zotero API key in the namespace or current user.
func CreateZoteroExport(target ZoteroExportTarget) (string, error) {
	url := "/api/v1/zotero/exports"

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(&target)
	if err != nil {
		return "", err
	}

	resp, err := client.MakeRequest("POST", url, &buf, "application/json")
	if err != nil {
		return "", fmt.Errorf("failed to create zotero exports: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create zotero exports: %s", string(body))
	}

	var exportResp struct {
		ExportID string `json:"exportId"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&exportResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return exportResp.ExportID, nil
}

// FetchLatestZoteroCollections fetches the latest version of Zotero items
// from TPIX server.
func FetchLatestZoteroCollections(exportID string, writer io.Writer) error {
	url := fmt.Sprintf("/api/v1/zotero/exports/%s", exportID)

	resp, err := client.MakeRequest("GET", url, nil, "")
	if err != nil {
		return fmt.Errorf("failed to delete zotero export: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to export zotero collections: %s", string(body))
	}

	_, err = io.Copy(writer, resp.Body)
	return err
}

func DeleteZoteroExport(exportID string) error {
	url := fmt.Sprintf("/api/v1/zotero/exports/%s", exportID)

	resp, err := client.MakeRequest("DELETE", url, nil, "")
	if err != nil {
		return fmt.Errorf("failed to delete zotero export: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete zotero export: %s", string(body))
	}

	return nil
}
