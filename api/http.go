package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
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

func readError(resp *http.Response) error {
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var reqErr RequestError
	err = json.Unmarshal(payload, &reqErr)
	if err != nil {
		reqErr.Message = string(payload)
	}

	reqErr.Code = resp.StatusCode
	return &reqErr
}

// SearchPackages fetches packages matching a query from the TPIX server.
// kind: "pkg", "template", or "all"
// sort: "name", "updated", or "popularity" (default)
func SearchPackages(query, namespace string, kind string, category string, sort string, limit int) (*SearchResponse, error) {
	path, _ := url.Parse("/api/v1/search")

	queries := url.Values{}
	if query != "" {
		queries.Add("q", query)
	}
	if namespace != "" {
		queries.Add("namespace", namespace)
	}
	if kind != "" {
		queries.Add("kind", kind)
	}
	if category != "" {
		queries.Add("category", category)
	}
	if sort != "" {
		queries.Add("sort", sort)
	}
	if limit > 0 {
		queries.Add("limit", fmt.Sprintf("%d", limit))
	}

	path.RawQuery = queries.Encode()

	resp, err := client.MakeRequest("GET", path.String(), nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to search packages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed: %w", readError(resp))
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// DownloadPackage downloads a package, extracts it to the cache directory,
// and saves the archive to output path.
func DownloadPackage(namespace, name, version string, cacheDir string) error {
	url := &url.URL{Path: fmt.Sprintf("/api/v1/download/%s/%s/%s", namespace, name, version)}

	resp, err := client.MakeRequest("GET", url.String(), nil, "")
	if err != nil {
		return fmt.Errorf("failed to download package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %w", readError(resp))
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
	path := &url.URL{Path: fmt.Sprintf("/api/v1/packages/%s/%s", namespace, name)}
	resp, err := client.MakeRequest("GET", path.String(), nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get package: %w", readError(resp))
	}

	var pkg PackageResponse
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &pkg, nil
}

// FetchDependencies fetches the dependencies for a specific package version.
func FetchDependencies(namespace, name, version string) ([]DependencyInfo, error) {
	path := &url.URL{Path: fmt.Sprintf("/api/v1/packages/%s/%s/%s/dependencies", namespace, name, version)}
	resp, err := client.MakeRequest("GET", path.String(), nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch dependencies: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get dependencies: %w", readError(resp))
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
	path := "/api/v1/packages/upload"
	resp, err := client.MakeRequest("POST", path, &buf, writer.FormDataContentType())
	if err != nil {
		return nil, fmt.Errorf("failed to upload package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("upload package failed: %w", readError(resp))
	}

	var uploadResp UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &uploadResp, nil
}

// QueryZoteroLibraries fetches zotero libraries the current user has access
// permission. This includes libraries granted in personal account, and libraries
// granted by the namespace owners.
func QueryZoteroLibraries() ([]ZoteroLibrary, error) {
	path := "/api/v1/zotero/libraries"
	resp, err := client.MakeRequest("GET", path, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch zotero libraries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch zotero libraries: %w", readError(resp))
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
	path := "/api/v1/zotero/exports"

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(&target)
	if err != nil {
		return "", err
	}

	resp, err := client.MakeRequest("POST", path, &buf, "application/json")
	if err != nil {
		return "", fmt.Errorf("failed to create zotero exports: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("failed to create zotero exports: %w", readError(resp))
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
	path := &url.URL{Path: fmt.Sprintf("/api/v1/zotero/exports/%s", exportID)}

	resp, err := client.MakeRequest("GET", path.String(), nil, "")
	if err != nil {
		return fmt.Errorf("failed to fetch zotero export: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to export zotero collections: %w", readError(resp))
	}

	_, err = io.Copy(writer, resp.Body)
	return fmt.Errorf("failed to read zotero export: %w", err)
}

func DeleteZoteroExport(exportID string) error {
	path := &url.URL{Path: fmt.Sprintf("/api/v1/zotero/exports/%s", exportID)}

	resp, err := client.MakeRequest("DELETE", path.String(), nil, "")
	if err != nil {
		return fmt.Errorf("failed to delete zotero export: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to delete zotero export: %w", readError(resp))
	}

	return nil
}

func GetUserProfile() (*UserProfile, error) {
	path := "/api/v1/profile"

	resp, err := client.MakeRequest("GET", path, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user profile: %w", readError(resp))
	}

	var profile UserProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &profile, nil
}

func GetPackageIndex() (string, error) {
	path := "/api/v1/llm.txt"

	resp, err := client.MakeRequest("GET", path, nil, "")
	if err != nil {
		return "", fmt.Errorf("failed to download llm.txt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get llm.txt: %w", readError(resp))
	}

	txt, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read llm.txt: %s", err)
	}

	return string(txt), nil
}
