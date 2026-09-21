package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	cli "github.com/typstify/tpix-cli"
	"github.com/typstify/tpix-cli/api"
	"github.com/typstify/tpix-cli/config"
	"github.com/typstify/tpix-cli/storage"
)

// withJSON enables or disables the global --json flag for the duration of a test.
func withJSON(t *testing.T, on bool) {
	t.Helper()
	orig := jsonFlag
	jsonFlag = on
	t.Cleanup(func() { jsonFlag = orig })
}

// tarGzArchive builds an in-memory tar.gz archive for download mocks.
func tarGzArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeImport(t *testing.T, dir, spec string) {
	t.Helper()
	content := "#import \"" + spec + "\": canvas"
	if err := os.WriteFile(filepath.Join(dir, "main.typ"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func decodeResult(t *testing.T, out []byte) (resultLine, map[string]any) {
	t.Helper()
	var line resultLine
	if err := json.Unmarshal(out, &line); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, out)
	}
	data, _ := line.Data.(map[string]any)
	return line, data
}

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, exitOK},
		{"generic", io.EOF, exitGeneric},
		{"usage", usageErrorf("bad"), exitUsage},
		{"auth", &api.RequestError{Code: 401, Message: "no"}, exitAuth},
		{"forbidden", &api.RequestError{Code: 403, Message: "no"}, exitAuth},
		{"notfound", &api.RequestError{Code: 404, Message: "no"}, exitNotFound},
		{"server", &api.RequestError{Code: 503, Message: "no"}, exitNetwork},
	}
	for _, tc := range cases {
		if got := exitCodeFor(tc.err); got != tc.want {
			t.Errorf("%s: exitCodeFor() = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestSearchCmdJSONOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SearchResponse{
			Query: "cetz",
			Total: 1,
			Count: 1,
			Results: []api.SearchResult{
				{Name: "cetz", Namespace: "preview", LatestVersion: "0.3.0"},
			},
		})
	}))
	defer srv.Close()

	httpClient := api.NewHttpClient(nil)
	httpClient.SetBaseURL(srv.URL)
	httpClient.SetMaxRetry(1)

	orig := sdk
	sdk = cli.NewTpixSdk(httpClient, nil)
	t.Cleanup(func() { sdk = orig })

	withJSON(t, true)

	cmd := searchPkgCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"cetz"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("search --format json error = %v", err)
	}

	// stdout must be exactly one JSON document with no progress pollution.
	line, data := decodeResult(t, out.Bytes())
	if line.Type != "result" || line.Command != "search" {
		t.Errorf("unexpected envelope: %+v", line)
	}
	if line.SchemaVersion != schemaVersion {
		t.Errorf("schemaVersion = %d, want %d", line.SchemaVersion, schemaVersion)
	}
	if data["query"] != "cetz" {
		t.Errorf("data.query = %v, want cetz", data["query"])
	}
}

func TestPullCmdJSONDryRun(t *testing.T) {
	store, err := storage.NewFsPackageStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	orig := sdk
	sdk = cli.NewTpixSdk(api.NewHttpClient(nil), store)
	t.Cleanup(func() { sdk = orig })

	dir := t.TempDir()
	writeImport(t, dir, "@preview/cetz:0.3.0")
	t.Chdir(dir)

	withJSON(t, true)

	cmd := pullCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("pull --dry-run --format json error = %v", err)
	}

	line, data := decodeResult(t, out.Bytes())
	if line.Command != "pull" {
		t.Errorf("command = %q, want pull", line.Command)
	}
	if data["dryRun"] != true {
		t.Errorf("data.dryRun = %v, want true", data["dryRun"])
	}

	direct, _ := data["direct"].([]any)
	if len(direct) != 1 {
		t.Fatalf("direct = %v, want 1 entry", data["direct"])
	}
	entry, _ := direct[0].(map[string]any)
	if entry["name"] != "cetz" || entry["cached"] != false {
		t.Errorf("unexpected direct entry: %v", entry)
	}

	// A dry run must not resolve any packages.
	if pkgs, _ := data["packages"].([]any); len(pkgs) != 0 {
		t.Errorf("packages = %v, want empty", data["packages"])
	}
}

func TestPullCmdJSONFetch(t *testing.T) {
	archive := tarGzArchive(t, map[string]string{"typst.toml": "x"})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/download/preview/cetz/0.3.0", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/api/v1/packages/preview/cetz/0.3.0/dependencies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.DependenciesResponse{Package: "cetz", Version: "0.3.0"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	httpClient := api.NewHttpClient(nil)
	httpClient.SetBaseURL(srv.URL)
	httpClient.SetMaxRetry(1)

	store, err := storage.NewFsPackageStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	orig := sdk
	sdk = cli.NewTpixSdk(httpClient, store)
	t.Cleanup(func() { sdk = orig })

	dir := t.TempDir()
	writeImport(t, dir, "@preview/cetz:0.3.0")
	t.Chdir(dir)

	withJSON(t, true)

	cmd := pullCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("pull --format json error = %v", err)
	}

	_, data := decodeResult(t, out.Bytes())
	if data["dryRun"] != false {
		t.Errorf("data.dryRun = %v, want false", data["dryRun"])
	}

	pkgs, _ := data["packages"].([]any)
	if len(pkgs) != 1 {
		t.Fatalf("packages = %v, want 1 entry", data["packages"])
	}
	entry, _ := pkgs[0].(map[string]any)
	if entry["name"] != "cetz" || entry["version"] != "0.3.0" {
		t.Errorf("unexpected package entry: %v", entry)
	}
	if entry["cached"] != false {
		t.Errorf("cached = %v, want false (newly downloaded)", entry["cached"])
	}
}

func TestPushCmdJSONOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/packages/upload" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.UploadResponse{
			SHA256:    "abc123",
			Namespace: "acme",
			Package:   "mypkg",
			Version:   "1.0.0",
			Size:      10,
		})
	}))
	defer srv.Close()

	httpClient := api.NewHttpClient(nil)
	httpClient.SetBaseURL(srv.URL)
	httpClient.SetMaxRetry(1)

	orig := sdk
	sdk = cli.NewTpixSdk(httpClient, nil)
	t.Cleanup(func() { sdk = orig })

	pkgFile := filepath.Join(t.TempDir(), "mypkg.tar.gz")
	if err := os.WriteFile(pkgFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	withJSON(t, true)

	cmd := pushCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{pkgFile, "acme"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("push --format json error = %v", err)
	}

	line, data := decodeResult(t, out.Bytes())
	if line.Command != "push" {
		t.Errorf("command = %q, want push", line.Command)
	}
	if data["success"] != true || data["sha256"] != "abc123" || data["version"] != "1.0.0" {
		t.Errorf("unexpected push result: %v", data)
	}
}

func TestZoteroExportJSONRequiresLibrary(t *testing.T) {
	withJSON(t, true)

	cmd := zoteroExportCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(nil)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected a usage error when --library is missing in json mode")
	}
	if got := exitCodeFor(err); got != exitUsage {
		t.Errorf("exit code = %d, want %d", got, exitUsage)
	}
}

func TestDepsCmdJSONOutput(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/packages/preview/cetz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.PackageResponse{
			Name: "cetz", Namespace: "preview",
			Versions: []api.PackageVersionInfo{{Version: "0.3.0"}},
		})
	})
	mux.HandleFunc("/api/v1/packages/preview/cetz/0.3.0/dependencies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.DependenciesResponse{
			Dependencies: []api.DependencyInfo{{Namespace: "preview", Name: "tablex", Version: "0.0.6"}},
		})
	})
	mux.HandleFunc("/api/v1/packages/preview/tablex/0.0.6/dependencies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.DependenciesResponse{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	httpClient := api.NewHttpClient(nil)
	httpClient.SetBaseURL(srv.URL)
	httpClient.SetMaxRetry(1)

	store, err := storage.NewFsPackageStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	orig := sdk
	sdk = cli.NewTpixSdk(httpClient, store)
	t.Cleanup(func() { sdk = orig })

	withJSON(t, true)

	cmd := depsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"@preview/cetz"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("deps --json error = %v", err)
	}

	line, data := decodeResult(t, out.Bytes())
	if line.Command != "deps" {
		t.Errorf("command = %q, want deps", line.Command)
	}
	root, _ := data["root"].(map[string]any)
	pkg, _ := root["package"].(map[string]any)
	if pkg["name"] != "cetz" {
		t.Errorf("root package = %v, want cetz", pkg)
	}
	children, _ := root["children"].([]any)
	if len(children) != 1 {
		t.Fatalf("children = %v, want 1 entry", root["children"])
	}
	child, _ := children[0].(map[string]any)
	childPkg, _ := child["package"].(map[string]any)
	if childPkg["name"] != "tablex" {
		t.Errorf("child package = %v, want tablex", childPkg)
	}
}

func TestEmitErrorJSON(t *testing.T) {
	withJSON(t, true)

	cmd := searchPkgCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	emitError(cmd, &api.RequestError{Code: 401, Message: "unauthorized", Description: "bad key"})

	var line errorLine
	if err := json.Unmarshal(out.Bytes(), &line); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
	}
	if line.Type != "error" {
		t.Errorf("type = %q, want error", line.Type)
	}
	if line.Error == nil {
		t.Fatal("missing error detail")
	}
	if line.Error.Code != exitAuth || line.Error.HTTPStatus != 401 {
		t.Errorf("unexpected error detail: %+v", line.Error)
	}
}

func TestWhoamiCmdJSONOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/profile" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.UserProfile{
			Username:   "alice",
			Email:      "alice@example.com",
			Namespaces: []api.UserNamespace{{Name: "acme", Permission: "write"}},
		})
	}))
	defer srv.Close()

	httpClient := api.NewHttpClient(nil)
	httpClient.SetBaseURL(srv.URL)
	httpClient.SetMaxRetry(1)

	orig := sdk
	sdk = cli.NewTpixSdk(httpClient, nil)
	t.Cleanup(func() { sdk = orig })

	withJSON(t, true)

	cmd := whoamiCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("whoami --json error = %v", err)
	}

	line, data := decodeResult(t, out.Bytes())
	if line.Command != "whoami" {
		t.Errorf("command = %q, want whoami", line.Command)
	}
	if data["username"] != "alice" || data["email"] != "alice@example.com" {
		t.Errorf("unexpected profile: %v", data)
	}
	ns, _ := data["namespaces"].([]any)
	if len(ns) != 1 {
		t.Fatalf("namespaces = %v, want 1 entry", data["namespaces"])
	}
}

func TestLogoutCmdJSONOutput(t *testing.T) {
	origDir := configDir
	configDir = t.TempDir()
	t.Cleanup(func() { configDir = origDir })

	origCm := cm
	cm = &CliConfigManager{}
	t.Cleanup(func() { cm = origCm })

	// Seed a stored key, then log out.
	if err := cm.Save(config.Config{
		Credentials:       config.Credentials{ApiKey: "secret"},
		TypstCachePkgPath: configDir,
	}); err != nil {
		t.Fatal(err)
	}

	withJSON(t, true)

	cmd := logoutCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("logout --json error = %v", err)
	}

	line, data := decodeResult(t, out.Bytes())
	if line.Command != "logout" || data["success"] != true {
		t.Errorf("unexpected logout result: %+v", line)
	}

	cfg, err := cm.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ApiKey != "" {
		t.Errorf("api key = %q, want empty after logout", cfg.ApiKey)
	}
}

func TestEmitErrorTextIsNoop(t *testing.T) {
	withJSON(t, false)

	cmd := searchPkgCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	emitError(cmd, io.EOF)

	if out.Len() != 0 {
		t.Errorf("text mode emitError wrote %q, want nothing", out.String())
	}
}
