package api

import (
	"time"

	"github.com/typstify/tpix-cli/bundler"
)

// API response types

type SearchResponse struct {
	Query   string         `json:"query"`
	Count   int            `json:"count"`
	Results []SearchResult `json:"results"`
}

// SearchResult represents a single search result
type SearchResult struct {
	Name          string    `json:"name"`
	Namespace     string    `json:"namespace"`
	Description   string    `json:"description"`
	LatestVersion string    `json:"latest_version"`
	PublishedAt   time.Time `json:"published_at"` // latest published time
	License       string    `json:"license"`
	IsTemplate    bool      `json:"is_template"`
	Authors       []string  `json:"authors"`
	Categories    []string  `json:"categories"`
	Disciplines   []string  `json:"disciplines"`
	CreatedAt     time.Time `json:"created_at"`
}

// PackageVersionInfo represents package version information
type PackageVersionInfo struct {
	Version     string           `json:"version"`
	SHA256      string           `json:"sha256"`
	PublishedAt time.Time        `json:"published_at"`
	Meta        bundler.Manifest `json:"meta"`
}

// PackageResponse represents a package details response
type PackageResponse struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	Namespace       string               `json:"namespace"`
	SourceType      string               `json:"source_type"`
	ExternalURL     string               `json:"external_url"`
	Description     string               `json:"description"`
	HomepageURL     string               `json:"homepage_url"`
	RepositoryURL   string               `json:"repository_url"`
	License         string               `json:"license"`
	IsTemplate      bool                 `json:"is_template"`
	LastPublishedAt time.Time            `json:"last_published_at"` // latest published time
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
	Versions        []PackageVersionInfo `json:"versions"`
}

// UploadResponse represents an upload response.
// When the package validation does not pass, only ValidateReport
// is returned.
type UploadResponse struct {
	SHA256         string   `json:"sha256"`
	Namespace      string   `json:"namespace"`
	Package        string   `json:"package"`
	Version        string   `json:"version"`
	Size           int64    `json:"size"`
	ValidateReport []string `json:"report"`
}

// DependencyInfo represents a single package dependency
type DependencyInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version"`
}

// DependenciesResponse represents the dependencies endpoint response
type DependenciesResponse struct {
	Package      string           `json:"package"`
	Version      string           `json:"version"`
	Dependencies []DependencyInfo `json:"dependencies"`
}

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"description"`
}

type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

type UserProfile struct {
	Email      string          `json:"email"`
	Username   string          `json:"username"`
	CreatedAt  time.Time       `json:"created_at"`
	Namespaces []UserNamespace `json:"namespaces"`
	Subscribed bool            `json:"subscribed"`
}

type UserNamespace struct {
	Name       string `json:"name"`
	Permission string `json:"permission"`
}

// ZoteroGroup represents a Zotero group
type ZoteroGroup struct {
	ID          int    `json:"id"`
	Version     int    `json:"version"`
	Name        string `json:"name"`
	Owner       int    `json:"owner"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Url         string `json:"url"`
}

// ZoteroCollection represents a Zotero collection
type ZoteroCollection struct {
	Key              string `json:"key"`
	Version          int    `json:"version"`
	Name             string `json:"name"`
	ParentCollection string `json:"parentCollection"`
}

type ZoteroLibrary struct {
	Namespace   string             `json:"namespace"`
	NamespaceID string             `json:"namespace_id"`
	Scope       string             `json:"scope"` // "users"||"groups"
	Library     ZoteroGroup        `json:"library"`
	Collections []ZoteroCollection `json:"collections"`
}

type ZoteroExportTarget struct {
	NamespaceID   string `json:"namespace_id"`
	Name          string `json:"name" binding:"required"`
	LibraryType   string `json:"library_type" binding:"required"`
	LibraryID     int64  `json:"library_id" binding:"required"`
	CollectionKey string `json:"collection_key"`
	Format        string `json:"format"`
}
