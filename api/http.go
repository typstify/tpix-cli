package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"

	"github.com/typstify/tpix-cli/deps"
	"github.com/typstify/tpix-cli/storage"
)

// ApiClient is the API wrapper to access TPIX rest APIs.
type ApiClient struct {
	client *HttpClient
	store  storage.PackageStore
}

func NewApiClient(client *HttpClient, store storage.PackageStore) *ApiClient {
	return &ApiClient{client: client, store: store}
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
func (c *ApiClient) SearchPackages(query, namespace string, kind string, category string, sort string, limit int) (*SearchResponse, error) {
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

	resp, err := c.client.MakeRequest("GET", path.String(), nil, "")
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

// DownloadPackage downloads a package, and save to the package store.
func (c *ApiClient) DownloadPackage(namespace, name, version string) error {
	if namespace == "" || name == "" || version == "" {
		return errors.New("missing namespace or name, or version")
	}

	url := &url.URL{Path: fmt.Sprintf("/api/v1/download/%s/%s/%s", namespace, name, version)}

	resp, err := c.client.MakeRequest("GET", url.String(), nil, "")
	if err != nil {
		return fmt.Errorf("failed to download package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %w", readError(resp))
	}

	if c.store == nil {
		return fmt.Errorf("typst cache storage not set")
	}

	pkg := deps.Dependency{Namespace: namespace, Name: name, Version: version}
	err = c.store.Add(pkg, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to add package %s to package store: %w", pkg, err)
	}

	return nil
}

// FetchPackage fetches package details from the TPIX server.
func (c *ApiClient) FetchPackage(namespace, name string) (*PackageResponse, error) {
	if namespace == "" || name == "" {
		return nil, errors.New("missing namespace or name")
	}

	path := &url.URL{Path: fmt.Sprintf("/api/v1/packages/%s/%s", namespace, name)}
	resp, err := c.client.MakeRequest("GET", path.String(), nil, "")
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
func (c *ApiClient) FetchDependencies(namespace, name, version string) ([]DependencyInfo, error) {
	if namespace == "" || name == "" || version == "" {
		return nil, errors.New("namespace, package name and version are required")
	}

	path := &url.URL{Path: fmt.Sprintf("/api/v1/packages/%s/%s/%s/dependencies", namespace, name, version)}
	resp, err := c.client.MakeRequest("GET", path.String(), nil, "")
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
func (c *ApiClient) UploadPackage(packagePath, namespace string) (*UploadResponse, error) {
	if packagePath == "" {
		return nil, errors.New("package path is empty")
	}
	if namespace == "" {
		return nil, errors.New("namespace is missing")
	}

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
	resp, err := c.client.MakeRequest("POST", path, &buf, writer.FormDataContentType())
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
func (c *ApiClient) QueryZoteroLibraries() ([]ZoteroLibrary, error) {
	path := "/api/v1/zotero/libraries"
	resp, err := c.client.MakeRequest("GET", path, nil, "")
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
func (c *ApiClient) CreateZoteroExport(target ZoteroExportTarget) (string, error) {
	path := "/api/v1/zotero/exports"

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(&target)
	if err != nil {
		return "", err
	}

	resp, err := c.client.MakeRequest("POST", path, &buf, "application/json")
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
func (c *ApiClient) FetchLatestZoteroCollections(exportID string, writer io.Writer) error {
	if exportID == "" {
		return errors.New("export id is missing")
	}

	path := &url.URL{Path: fmt.Sprintf("/api/v1/zotero/exports/%s", exportID)}

	resp, err := c.client.MakeRequest("GET", path.String(), nil, "")
	if err != nil {
		return fmt.Errorf("failed to fetch zotero export: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to export zotero collections: %w", readError(resp))
	}

	_, err = io.Copy(writer, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read zotero export: %w", err)
	}

	return nil
}

func (c *ApiClient) DeleteZoteroExport(exportID string) error {
	if exportID == "" {
		return errors.New("export id is missing")
	}

	path := &url.URL{Path: fmt.Sprintf("/api/v1/zotero/exports/%s", exportID)}

	resp, err := c.client.MakeRequest("DELETE", path.String(), nil, "")
	if err != nil {
		return fmt.Errorf("failed to delete zotero export: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to delete zotero export: %w", readError(resp))
	}

	return nil
}

func (c *ApiClient) GetUserProfile() (*UserProfile, error) {
	path := "/api/v1/profile"

	resp, err := c.client.MakeRequest("GET", path, nil, "")
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

func (c *ApiClient) GetPackageIndex() (string, error) {
	path := "/api/v1/llm.txt"

	resp, err := c.client.MakeRequest("GET", path, nil, "")
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

func (c *ApiClient) GetNamespacePackages(namespace string) ([]PackageResponse, error) {
	if namespace == "" {
		return nil, errors.New("namespace is missing")
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/index.json", namespace)

	resp, err := c.client.MakeRequest("GET", path, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace index")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get namespace index: %w", readError(resp))
	}

	packages := []PackageResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&packages); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return packages, nil
}
