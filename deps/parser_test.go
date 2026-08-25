package deps

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFromSource(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []Dependency
	}{
		{
			name:    "single import",
			content: `#import "@preview/cetz:0.3.0": canvas`,
			want: []Dependency{
				{Namespace: "preview", Name: "cetz", Version: "0.3.0"},
			},
		},
		{
			name: "multiple imports",
			content: `#import "@preview/cetz:0.3.0": canvas
#import "@preview/tablex:0.0.6": tablex`,
			want: []Dependency{
				{Namespace: "preview", Name: "cetz", Version: "0.3.0"},
				{Namespace: "preview", Name: "tablex", Version: "0.0.6"},
			},
		},
		{
			name: "duplicate imports",
			content: `#import "@preview/cetz:0.3.0": canvas
#import "@preview/cetz:0.3.0": draw`,
			want: []Dependency{
				{Namespace: "preview", Name: "cetz", Version: "0.3.0"},
			},
		},
		{
			name:    "commented out import (line comment)",
			content: `// #import "@preview/cetz:0.3.0": canvas`,
			want:    nil,
		},
		{
			name:    "commented out import (block comment)",
			content: `/* #import "@preview/cetz:0.3.0": canvas */`,
			want:    nil,
		},
		{
			name: "block comment spanning multiple lines",
			content: `/*
#import "@preview/cetz:0.3.0": canvas
*/
#import "@preview/tablex:0.0.6": tablex`,
			want: []Dependency{
				{Namespace: "preview", Name: "tablex", Version: "0.0.6"},
			},
		},
		{
			name:    "no imports",
			content: `= Hello World`,
			want:    nil,
		},
		{
			name: "different namespaces",
			content: `#import "@preview/cetz:0.3.0": canvas
#import "@myns/mypkg:1.0.0": *`,
			want: []Dependency{
				{Namespace: "preview", Name: "cetz", Version: "0.3.0"},
				{Namespace: "myns", Name: "mypkg", Version: "1.0.0"},
			},
		},
		{
			name:    "import with inline comment after",
			content: `#import "@preview/cetz:0.3.0": canvas // some comment`,
			want: []Dependency{
				{Namespace: "preview", Name: "cetz", Version: "0.3.0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractFromSource([]byte(tt.content))
			if len(got) != len(tt.want) {
				t.Fatalf("ExtractFromSource() returned %d deps, want %d", len(got), len(tt.want))
			}
			for i, dep := range got {
				if dep != tt.want[i] {
					t.Errorf("dep[%d] = %+v, want %+v", i, dep, tt.want[i])
				}
			}
		})
	}
}

func TestExtractFromDirectory(t *testing.T) {
	// Create temp directory with .typ files
	tmpDir := t.TempDir()

	// Write a .typ file with imports
	content1 := `#import "@preview/cetz:0.3.0": canvas
#import "@preview/tablex:0.0.6": tablex
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.typ"), []byte(content1), 0644); err != nil {
		t.Fatal(err)
	}

	// Write another .typ file in a subdirectory
	subDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	content2 := `#import "@preview/cetz:0.3.0": draw
#import "@myns/utils:1.0.0": *
`
	if err := os.WriteFile(filepath.Join(subDir, "helpers.typ"), []byte(content2), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a non-.typ file (should be ignored)
	if err := os.WriteFile(filepath.Join(tmpDir, "notes.txt"), []byte(`#import "@preview/ignored:0.0.1"`), 0644); err != nil {
		t.Fatal(err)
	}

	deps, err := ExtractFromDirectory(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Should have 3 unique deps: cetz, tablex, utils (cetz deduplicated)
	if len(deps) != 3 {
		t.Fatalf("got %d deps, want 3: %+v", len(deps), deps)
	}

	// Verify the deps exist (order not guaranteed due to filepath.Walk)
	found := make(map[string]bool)
	for _, dep := range deps {
		found[dep.String()] = true
	}

	expected := []string{
		"@preview/cetz:0.3.0",
		"@preview/tablex:0.0.6",
		"@myns/utils:1.0.0",
	}
	for _, key := range expected {
		if !found[key] {
			t.Errorf("missing expected dependency: %s", key)
		}
	}
}

func TestDependencyKey(t *testing.T) {
	dep := Dependency{Namespace: "preview", Name: "cetz", Version: "0.3.0"}
	want := "@preview/cetz:0.3.0"
	if got := dep.String(); got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
}
