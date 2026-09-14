package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	cli "github.com/typstify/tpix-cli"
	"github.com/typstify/tpix-cli/api"
	"github.com/typstify/tpix-cli/deps"
	"github.com/typstify/tpix-cli/storage"
)

func withStore(t *testing.T, store *storage.FsPackageStore) {
	t.Helper()

	orig := pkgCache
	pkgCache = store
	t.Cleanup(func() { pkgCache = orig })
}

func withSilentReporter(t *testing.T) {
	t.Helper()

	orig := cmdReporter
	cmdReporter = func(string) {}
	t.Cleanup(func() { cmdReporter = orig })
}

func TestRemoveCachedCmdExisting(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewFsPackageStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	withStore(t, store)
	withSilentReporter(t)

	spec := deps.Dependency{Namespace: "preview", Name: "cetz", Version: "0.3.0"}
	if err := os.MkdirAll(filepath.Join(dir, "preview", "cetz", "0.3.0"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := removeCachedCmd()
	cmd.SetArgs([]string{"preview/cetz:0.3.0"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remove existing package error = %v", err)
	}

	if got, err := store.Has(spec); err != nil || got {
		t.Fatalf("Has() after remove = (%v, %v), want (false, nil)", got, err)
	}
}

func TestRemoveCachedCmdMissingReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewFsPackageStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	withStore(t, store)
	withSilentReporter(t)

	cmd := removeCachedCmd()
	cmd.SetArgs([]string{"preview/nope:1.0.0"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); err == nil {
		t.Fatal("removing a missing package should return an error, got nil")
	}
}

func TestRemoveCachedCmdInvalidSpecReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewFsPackageStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	withStore(t, store)
	withSilentReporter(t)

	cmd := removeCachedCmd()
	cmd.SetArgs([]string{"foo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); err == nil {
		t.Fatal("removing with an invalid spec should return an error, got nil")
	}
}

func TestSearchCmdReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	httpClient := api.NewHttpClient(nil)
	httpClient.SetBaseURL(srv.URL)

	orig := sdk
	sdk = cli.NewTpixSdk(httpClient, nil)
	t.Cleanup(func() { sdk = orig })

	cmd := searchPkgCmd()
	cmd.SetArgs([]string{"hello"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); err == nil {
		t.Fatal("search should return an error when the server fails, got nil")
	}
}
