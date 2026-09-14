package utils

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type archiveEntry struct {
	name string
	body string
	dir  bool
}

func buildTarGz(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Typeflag: tar.TypeReg}
		if e.dir {
			hdr.Typeflag = tar.TypeDir
			hdr.Mode = 0o755
		} else {
			hdr.Size = int64(len(e.body))
		}

		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header %q: %v", e.name, err)
		}
		if !e.dir {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("write tar body %q: %v", e.name, err)
			}
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

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}

func TestExtractTarGzFromReader(t *testing.T) {
	dest := t.TempDir()
	archive := buildTarGz(t, []archiveEntry{
		{name: "typst.toml", body: "[package]"},
		{name: "src/", dir: true},
		{name: "src/main.typ", body: "= Hello"},
	})

	if err := ExtractTarGzFromReader(bytes.NewReader(archive), dest); err != nil {
		t.Fatalf("ExtractTarGzFromReader() error = %v", err)
	}

	assertFileContent(t, filepath.Join(dest, "typst.toml"), "[package]")
	assertFileContent(t, filepath.Join(dest, "src", "main.typ"), "= Hello")
}

func TestExtractTarGzFromReaderRejectsZipSlip(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "pkg")

	archive := buildTarGz(t, []archiveEntry{
		{name: "ok.txt", body: "ok"},
		{name: "../escaped.txt", body: "pwned"},
	})

	if err := ExtractTarGzFromReader(bytes.NewReader(archive), dest); err == nil {
		t.Fatal("expected error for path-traversing archive entry, got nil")
	}

	if _, err := os.Stat(filepath.Join(parent, "escaped.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("archive entry escaped destination directory: stat error = %v", err)
	}
}

func TestExtractTarGzFromReaderRejectsAbsolutePath(t *testing.T) {
	dest := t.TempDir()
	archive := buildTarGz(t, []archiveEntry{
		{name: "/tmp/tpix-absolute-escape.txt", body: "pwned"},
	})

	if err := ExtractTarGzFromReader(bytes.NewReader(archive), dest); err == nil {
		t.Fatal("expected error for absolute archive entry, got nil")
	}

	if _, err := os.Stat("/tmp/tpix-absolute-escape.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("absolute archive entry was written outside destination: stat error = %v", err)
	}
}
