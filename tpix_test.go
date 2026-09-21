package tpix

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/typstify/tpix-cli/api"
	"github.com/typstify/tpix-cli/deps"
	"github.com/typstify/tpix-cli/storage"
)

// fakeKeyProvider is a minimal api.ApiKeyProvider for tests.
type fakeKeyProvider struct{ key string }

func (f fakeKeyProvider) Get() string { return f.key }

// newTestSdk builds a TpixSdk whose HTTP client is pointed at a test server
// serving the given handler.
func newTestSdk(t *testing.T, handler http.Handler) *TpixSdk {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	hc := api.NewHttpClient(fakeKeyProvider{key: "test-key"})
	hc.SetMaxRetry(1)
	hc.SetBaseURL(server.URL)

	store, err := storage.NewFsPackageStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFsPackageStore() error = %v", err)
	}

	return NewTpixSdk(hc, store)
}

// tarGzBytes builds an in-memory tar.gz archive from a map of file name to content.
func tarGzBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writePackageDir creates a directory with a typst.toml manifest and the given
// files (the manifest itself is written from the manifest argument).
func writePackageDir(t *testing.T, dir, manifest string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "typst.toml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBundlePackage(t *testing.T) {
	sdk := &TpixSdk{}

	// Missing directory.
	if _, err := sdk.BundlePackage(filepath.Join(t.TempDir(), "nope"), "", nil); err == nil {
		t.Error("expected error for missing directory")
	}

	// Directory without typst.toml.
	emptyDir := t.TempDir()
	if _, err := sdk.BundlePackage(emptyDir, "", nil); err == nil {
		t.Error("expected error for missing typst.toml")
	}

	// Valid package.
	manifest := `[package]
name = "mypkg"
version = "0.1.0"
entrypoint = "main.typ"
authors = ["test"]
license = "MIT"
description = "test"
`
	srcDir := filepath.Join(t.TempDir(), "mypkg")
	writePackageDir(t, srcDir, manifest, map[string]string{
		"main.typ":    "#import \"lib.typ\"\n= Hello\n",
		"src/lib.typ": "#let x = 1\n",
	})

	outputFile := filepath.Join(t.TempDir(), "mypkg.tar.gz")
	got, err := sdk.BundlePackage(srcDir, outputFile, nil)
	if err != nil {
		t.Fatalf("BundlePackage() error = %v", err)
	}
	if got != outputFile {
		t.Errorf("BundlePackage() = %q, want %q", got, outputFile)
	}
	if info, err := os.Stat(outputFile); err != nil || info.Size() == 0 {
		t.Errorf("expected non-empty output archive, got err=%v size=%v", err, info)
	}
}

func TestDownloadProjectDependenciesDryRun(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	sdk := newTestSdk(t, handler)

	// Directory with no imports should succeed without network.
	dir := t.TempDir()
	if _, err := sdk.DownloadProjectDependencies(dir, false); err != nil {
		t.Fatalf("DownloadProjectDependencies() error = %v", err)
	}

	// Dry run should report discovered deps without hitting the network.
	writePackageDir(t, dir, "", map[string]string{
		"main.typ": `#import "@preview/cetz:0.3.0": canvas`,
	})
	var msgs []string
	sdk.WithReporter(func(m string) { msgs = append(msgs, m) })
	if _, err := sdk.DownloadProjectDependencies(dir, true); err != nil {
		t.Fatalf("DownloadProjectDependencies() dry run error = %v", err)
	}
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "@preview/cetz:0.3.0") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected reporter to mention dependency, got %v", msgs)
	}
}

func TestSearchPackages(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/search" {
			t.Errorf("path = %s, want /api/v1/search", r.URL.Path)
		}
		if q := r.URL.Query().Get("q"); q != "cetz" {
			t.Errorf("q = %q, want cetz", q)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.SearchResponse{
			Query: "cetz",
			Total: 1,
			Count: 1,
			Results: []api.SearchResult{
				{Name: "cetz", Namespace: "preview", LatestVersion: "0.3.0"},
			},
		})
	})

	sdk := newTestSdk(t, handler)
	resp, err := sdk.SearchPackages("preview", "cetz", "pkg", "", "", 10)
	if err != nil {
		t.Fatalf("SearchPackages() error = %v", err)
	}
	if resp.Total != 1 || len(resp.Results) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Results[0].Name != "cetz" {
		t.Errorf("result name = %q, want cetz", resp.Results[0].Name)
	}
}

func TestQueryPackage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/packages/preview/cetz" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.PackageResponse{
			Name:      "cetz",
			Namespace: "preview",
			Versions:  []api.PackageVersionInfo{{Version: "0.3.0"}},
		})
	})

	sdk := newTestSdk(t, handler)
	pkg, err := sdk.QueryPackage("@preview/cetz")
	if err != nil {
		t.Fatalf("QueryPackage() error = %v", err)
	}
	if pkg.Name != "cetz" || len(pkg.Versions) != 1 {
		t.Errorf("unexpected package: %+v", pkg)
	}
}

func TestDownloadPackageResolvesLatestVersion(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/packages/preview/cetz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.PackageResponse{
			Name:      "cetz",
			Namespace: "preview",
			Versions: []api.PackageVersionInfo{
				{Version: "0.3.0"},
				{Version: "0.2.0"},
			},
		})
	})
	mux.HandleFunc("/api/v1/download/preview/cetz/0.3.0", func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarGzBytes(t, map[string]string{"typst.toml": "hello"}))
	})

	sdk := newTestSdk(t, mux)
	specs, err := sdk.DownloadPackage("@preview/cetz", true)
	if err != nil {
		t.Fatalf("DownloadPackage() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("resolved %d packages, want 1: %+v", len(specs), specs)
	}
	if specs[0].Package.Version != "0.3.0" {
		t.Errorf("resolved version = %q, want 0.3.0", specs[0].Package.Version)
	}
}

func TestDownloadPackageWithDeps(t *testing.T) {
	cetzArchive := tarGzBytes(t, map[string]string{
		"typst.toml": "hello",
		"main.typ":   "#import \"lib.typ\"\n",
	})
	tablexArchive := tarGzBytes(t, map[string]string{
		"typst.toml": "hello",
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/download/preview/cetz/0.3.0", func(w http.ResponseWriter, r *http.Request) {
		w.Write(cetzArchive)
	})
	mux.HandleFunc("/api/v1/download/preview/tablex/0.0.6", func(w http.ResponseWriter, r *http.Request) {
		w.Write(tablexArchive)
	})
	mux.HandleFunc("/api/v1/packages/preview/cetz/0.3.0/dependencies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.DependenciesResponse{
			Package:      "cetz",
			Version:      "0.3.0",
			Dependencies: []api.DependencyInfo{{Namespace: "preview", Name: "tablex", Version: "0.0.6"}},
		})
	})
	mux.HandleFunc("/api/v1/packages/preview/tablex/0.0.6/dependencies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.DependenciesResponse{
			Package: "tablex",
			Version: "0.0.6",
		})
	})

	sdk := newTestSdk(t, mux)
	var msgs []string
	sdk.WithReporter(func(m string) { msgs = append(msgs, m) })

	specs, err := sdk.DownloadPackage("@preview/cetz:0.3.0", false)
	if err != nil {
		t.Fatalf("DownloadPackage() error = %v", err)
	}

	// Both the package and its transitive dependency should be resolved.
	if len(specs) != 2 {
		t.Fatalf("resolved %d packages, want 2: %+v", len(specs), specs)
	}

	// Archives should be exists in the cache.
	if exists, err := sdk.store.Has(deps.ParseDependency("@preview/cetz:0.3.0")); err != nil || !exists {
		t.Errorf("cetz not resolved: %v", err)
	}

	if exists, err := sdk.store.Has(deps.ParseDependency("@preview/tablex:0.0.6")); err != nil || !exists {
		t.Errorf("tablex not resolved: %v", err)
	}

	// Reporter should announce both packages.
	joined := strings.Join(msgs, "")
	if !strings.Contains(joined, "@preview/cetz:0.3.0") || !strings.Contains(joined, "@preview/tablex:0.0.6") {
		t.Errorf("expected reporter to mention both packages, got: %s", joined)
	}
}

func TestGetUserProfile(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/profile" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.UserProfile{
			Email:    "user@example.com",
			Username: "user",
		})
	})

	sdk := newTestSdk(t, handler)
	profile, err := sdk.GetUserProfile()
	if err != nil {
		t.Fatalf("GetUserProfile() error = %v", err)
	}
	if profile.Username != "user" {
		t.Errorf("username = %q, want user", profile.Username)
	}
}

func TestGetPackageIndex(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/llm.txt" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("## Available packages\n"))
	})

	sdk := newTestSdk(t, handler)
	idx, err := sdk.GetPackageIndex()
	if err != nil {
		t.Fatalf("GetPackageIndex() error = %v", err)
	}
	if !strings.Contains(idx, "Available packages") {
		t.Errorf("unexpected index: %q", idx)
	}
}

func TestZoteroExportFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/zotero/libraries", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]api.ZoteroLibrary{
			{Namespace: "acme", Scope: "users", Library: api.ZoteroGroup{ID: 1, Name: "My Library"}},
		})
	})
	mux.HandleFunc("/api/v1/zotero/exports", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"exportId": "exp_123"})
	})
	mux.HandleFunc("/api/v1/zotero/exports/exp_123", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte("biblio"))
		case http.MethodDelete:
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected method: %s", r.Method)
		}
	})

	sdk := newTestSdk(t, mux)

	libs, err := sdk.ListZoteroLibraries()
	if err != nil {
		t.Fatalf("ListZoteroLibraries() error = %v", err)
	}
	if len(libs) != 1 || libs[0].Namespace != "acme" {
		t.Errorf("unexpected libraries: %+v", libs)
	}

	exportID, err := sdk.CreateZoteroExport("test", "ns_1", "users", 1, "", "biblatex")
	if err != nil {
		t.Fatalf("CreateZoteroExport() error = %v", err)
	}
	if exportID != "exp_123" {
		t.Errorf("exportID = %q, want exp_123", exportID)
	}

	var buf bytes.Buffer
	if err := sdk.FetchZoteroExport(exportID, &buf); err != nil {
		t.Fatalf("FetchZoteroExport() error = %v", err)
	}
	if buf.String() != "biblio" {
		t.Errorf("fetched = %q, want biblio", buf.String())
	}

	if err := sdk.DeleteZoteroExport(exportID); err != nil {
		t.Fatalf("DeleteZoteroExport() error = %v", err)
	}
}

func TestResolveDependencies(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/packages/preview/cetz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.PackageResponse{
			Name:      "cetz",
			Namespace: "preview",
			Versions: []api.PackageVersionInfo{
				{Version: "0.3.0"},
				{Version: "0.2.0"},
			},
		})
	})
	mux.HandleFunc("/api/v1/packages/preview/cetz/0.3.0/dependencies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.DependenciesResponse{
			Package:      "cetz",
			Version:      "0.3.0",
			Dependencies: []api.DependencyInfo{{Namespace: "preview", Name: "tablex", Version: "0.0.6"}},
		})
	})
	mux.HandleFunc("/api/v1/packages/preview/tablex/0.0.6/dependencies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.DependenciesResponse{Package: "tablex", Version: "0.0.6"})
	})

	sdk := newTestSdk(t, mux)
	graph, err := sdk.ResolveDependencies("@preview/cetz")
	if err != nil {
		t.Fatalf("ResolveDependencies() error = %v", err)
	}

	if graph.Root.Package.Version != "0.3.0" {
		t.Errorf("root version = %q, want 0.3.0", graph.Root.Package.Version)
	}
	if len(graph.Root.Children) != 1 || graph.Root.Children[0].Package.Name != "tablex" {
		t.Fatalf("unexpected children: %+v", graph.Root.Children)
	}
	if len(graph.Packages) != 2 {
		t.Errorf("flat packages = %d, want 2", len(graph.Packages))
	}
}
