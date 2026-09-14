package storage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/typstify/tpix-cli/deps"
)

func newTestStore(t *testing.T) *FsPackageStore {
	t.Helper()

	store, err := NewFsPackageStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFsPackageStore() error = %v", err)
	}

	return store
}

func tarGz(t *testing.T, files map[string]string) io.Reader {
	t.Helper()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	for name, body := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %q: %v", name, err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write body %q: %v", name, err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}

	return bytes.NewReader(buf.Bytes())
}

func dep(namespace, name, version string) deps.Dependency {
	return deps.Dependency{Namespace: namespace, Name: name, Version: version}
}

func mustAdd(t *testing.T, store *FsPackageStore, spec deps.Dependency, files map[string]string) {
	t.Helper()

	if err := store.Add(spec, tarGz(t, files)); err != nil {
		t.Fatalf("Add(%s) error = %v", spec, err)
	}
}

func assertSpecs(t *testing.T, got []deps.Dependency, want []string) {
	t.Helper()

	gotStrings := make([]string, 0, len(got))
	for _, spec := range got {
		gotStrings = append(gotStrings, fmt.Sprintf("%s/%s:%s", spec.Namespace, spec.Name, spec.Version))
	}

	sort.Strings(gotStrings)
	sort.Strings(want)

	if !slices.Equal(gotStrings, want) {
		t.Errorf("specs = %v, want %v", gotStrings, want)
	}
}

func TestFsPackageStoreAddHasPath(t *testing.T) {
	store := newTestStore(t)
	spec := dep("preview", "cetz", "0.3.0")

	if got, err := store.Has(spec); err != nil || got {
		t.Fatalf("Has() before Add = (%v, %v), want (false, nil)", got, err)
	}

	mustAdd(t, store, spec, map[string]string{"typst.toml": "[package]", "main.typ": "= cetz"})

	got, err := store.Has(spec)
	if err != nil {
		t.Fatalf("Has() error = %v", err)
	}
	if !got {
		t.Fatal("Has() after Add = false, want true")
	}

	path, err := store.Path(spec)
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	want := filepath.Join(store.cacheDir, "preview", "cetz", "0.3.0")
	if path != want {
		t.Errorf("Path() = %q, want %q", path, want)
	}

	content, err := os.ReadFile(filepath.Join(path, "main.typ"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "= cetz" {
		t.Errorf("main.typ = %q, want %q", content, "= cetz")
	}
}

func TestFsPackageStoreAddInvalidArchiveLeavesNoTrace(t *testing.T) {
	store := newTestStore(t)
	spec := dep("preview", "cetz", "0.3.0")

	if err := store.Add(spec, strings.NewReader("this is not a gzip archive")); err == nil {
		t.Fatal("Add() with invalid archive succeeded, want error")
	}

	if got, err := store.Has(spec); err != nil || got {
		t.Fatalf("Has() after failed Add = (%v, %v), want (false, nil)", got, err)
	}

	pkgDir := filepath.Join(store.cacheDir, "preview", "cetz")
	entries, err := os.ReadDir(pkgDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".pkg-staging-") {
			t.Errorf("staging directory %q left behind", entry.Name())
		}
	}

	pkgs, err := store.List(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 0 {
		t.Errorf("List() = %v, want empty", pkgs)
	}
}

func TestFsPackageStoreAddOverwritesExisting(t *testing.T) {
	store := newTestStore(t)
	spec := dep("preview", "cetz", "0.3.0")

	mustAdd(t, store, spec, map[string]string{"main.typ": "v1", "stale.typ": "gone"})
	mustAdd(t, store, spec, map[string]string{"main.typ": "v2"})

	dir, err := store.Path(spec)
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "main.typ"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "v2" {
		t.Errorf("main.typ = %q, want %q", content, "v2")
	}

	if _, err := os.Stat(filepath.Join(dir, "stale.typ")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stale.typ still present after overwrite: %v", err)
	}
}

func TestFsPackageStoreList(t *testing.T) {
	store := newTestStore(t)
	mustAdd(t, store, dep("preview", "cetz", "0.3.0"), map[string]string{"main.typ": "a"})
	mustAdd(t, store, dep("preview", "cetz", "0.4.0"), map[string]string{"main.typ": "b"})
	mustAdd(t, store, dep("local", "my-pkg", "1.0.0"), map[string]string{"main.typ": "c"})

	all, err := store.List(nil)
	if err != nil {
		t.Fatal(err)
	}
	assertSpecs(t, all, []string{"local/my-pkg:1.0.0", "preview/cetz:0.3.0", "preview/cetz:0.4.0"})

	filtered, err := store.List(func(spec deps.Dependency) bool { return spec.Namespace == "preview" })
	if err != nil {
		t.Fatal(err)
	}
	assertSpecs(t, filtered, []string{"preview/cetz:0.3.0", "preview/cetz:0.4.0"})
}

func TestFsPackageStoreRemove(t *testing.T) {
	store := newTestStore(t)
	v1 := dep("preview", "cetz", "0.3.0")
	v2 := dep("preview", "cetz", "0.4.0")
	mustAdd(t, store, v1, map[string]string{"main.typ": "a"})
	mustAdd(t, store, v2, map[string]string{"main.typ": "b"})

	if err := store.Remove(v1); err != nil {
		t.Fatalf("Remove(%s) error = %v", v1, err)
	}
	if got, err := store.Has(v1); err != nil || got {
		t.Errorf("Has(%s) after Remove = (%v, %v), want (false, nil)", v1, got, err)
	}
	if got, err := store.Has(v2); err != nil || !got {
		t.Errorf("Has(%s) after removing sibling version = (%v, %v), want (true, nil)", v2, got, err)
	}

	// A version-less spec removes every version of the package.
	if err := store.Remove(dep("preview", "cetz", "")); err != nil {
		t.Fatalf("Remove(all versions) error = %v", err)
	}
	if got, err := store.Has(v2); err != nil || got {
		t.Errorf("Has(%s) after removing all versions = (%v, %v), want (false, nil)", v2, got, err)
	}

	// Removing a missing package is a no-op.
	if err := store.Remove(v1); err != nil {
		t.Errorf("Remove(missing) error = %v, want nil", err)
	}
}

func TestFsPackageStoreInvalidSpec(t *testing.T) {
	store := newTestStore(t)
	invalid := deps.Dependency{}

	if _, err := store.Path(invalid); err == nil {
		t.Error("Path(invalid) = nil error, want error")
	}
	if _, err := store.Has(invalid); err == nil {
		t.Error("Has(invalid) = nil error, want error")
	}
	if err := store.Add(invalid, tarGz(t, map[string]string{"main.typ": "x"})); err == nil {
		t.Error("Add(invalid) = nil error, want error")
	}
	if _, err := store.View(invalid); err == nil {
		t.Error("View(invalid) = nil error, want error")
	}
	if err := store.Remove(invalid); err == nil {
		t.Error("Remove(invalid) = nil error, want error")
	}
}

func TestFsPackageViewRead(t *testing.T) {
	store := newTestStore(t)
	spec := dep("preview", "cetz", "0.3.0")
	mustAdd(t, store, spec, map[string]string{
		"typst.toml":   "[package]",
		"src/main.typ": "= cetz",
	})

	view, err := store.View(spec)
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	defer view.Close()

	data, err := fs.ReadFile(view, "typst.toml")
	if err != nil {
		t.Fatalf("ReadFile(typst.toml) error = %v", err)
	}
	if string(data) != "[package]" {
		t.Errorf("typst.toml = %q, want %q", data, "[package]")
	}

	entries, err := fs.ReadDir(view, "src")
	if err != nil {
		t.Fatalf("ReadDir(src) error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "main.typ" {
		t.Errorf("ReadDir(src) = %v, want [main.typ]", entries)
	}
}

func TestFsPackageViewIsSandboxed(t *testing.T) {
	store := newTestStore(t)
	viewed := dep("preview", "cetz", "0.3.0")
	other := dep("preview", "other", "0.1.0")
	mustAdd(t, store, viewed, map[string]string{"main.typ": "viewed"})
	mustAdd(t, store, other, map[string]string{"secret.typ": "other"})

	view, err := store.View(viewed)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Close()

	for _, name := range []string{
		"../other/0.1.0/secret.typ",
		"../../../../etc/passwd",
		"/etc/passwd",
	} {
		if _, err := view.Open(name); err == nil {
			t.Errorf("Open(%q) succeeded, want sandbox error", name)
		}
	}
}

func TestFsPackageViewMissingPackageReleasesLock(t *testing.T) {
	store := newTestStore(t)
	spec := dep("preview", "absent", "0.0.1")

	// The error path must release the read lock; the old bug panicked here.
	if _, err := store.View(spec); err == nil {
		t.Fatal("View(missing) = nil error, want error")
	}

	// If View leaked the read lock on failure, this Add would block forever.
	archive := tarGz(t, map[string]string{"main.typ": "x"})
	done := make(chan error, 1)
	go func() { done <- store.Add(spec, archive) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Add() after failed View error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Add() blocked; View leaked its read lock on the error path")
	}
}

func TestFsPackageViewCloseIsIdempotent(t *testing.T) {
	store := newTestStore(t)
	spec := dep("preview", "cetz", "0.3.0")
	mustAdd(t, store, spec, map[string]string{"main.typ": "x"})

	view, err := store.View(spec)
	if err != nil {
		t.Fatal(err)
	}

	if err := view.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := view.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestFsPackageViewBlocksAddUntilClose(t *testing.T) {
	store := newTestStore(t)
	spec := dep("preview", "cetz", "0.3.0")
	mustAdd(t, store, spec, map[string]string{"main.typ": "v1"})

	view, err := store.View(spec)
	if err != nil {
		t.Fatal(err)
	}

	archive := tarGz(t, map[string]string{"main.typ": "v2"})
	done := make(chan error, 1)
	go func() { done <- store.Add(spec, archive) }()

	select {
	case err := <-done:
		t.Fatalf("Add() completed while a view was open (err = %v)", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := view.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Add() did not complete after the view was closed")
	}
}

func TestFsPackageViewAllowsConcurrentReaders(t *testing.T) {
	store := newTestStore(t)
	spec := dep("preview", "cetz", "0.3.0")
	mustAdd(t, store, spec, map[string]string{"main.typ": "x"})

	first, err := store.View(spec)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	second, err := store.View(spec)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	if got, err := store.Has(spec); err != nil || !got {
		t.Fatalf("Has() with open views = (%v, %v), want (true, nil)", got, err)
	}
}
