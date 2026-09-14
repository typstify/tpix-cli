package deps

import (
	"path/filepath"
	"testing"
)

func TestDependencyIsValid(t *testing.T) {
	cases := []struct {
		name   string
		dep    Dependency
		expect bool
	}{
		{"only namespace", Dependency{Namespace: "ns-01"}, false},
		{"namespace and package", Dependency{Namespace: "ns-01", Name: "pkg-test"}, true},
		{"all field", Dependency{Namespace: "ns-01", Name: "pkg-test", Version: "v0.1.1"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.dep.IsValid() != tc.expect {
				t.Fail()
			}
		})
	}
}

func TestDependencyState(t *testing.T) {
	cases := []struct {
		name  string
		dep   Dependency
		state state
	}{
		{"empty", Dependency{}, stateInvalid},
		{"only namespace", Dependency{Namespace: "ns-01"}, stateInvalid},
		{"only name", Dependency{Name: "pkg-test"}, stateInvalid},
		{"namespace and package", Dependency{Namespace: "ns-01", Name: "pkg-test"}, statePartial},
		{"all fields", Dependency{Namespace: "ns-01", Name: "pkg-test", Version: "v0.1.1"}, stateComplete},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dep := tc.dep

			if got := dep.state(); got != tc.state {
				t.Fatalf("State() = %d, want %d", got, tc.state)
			}
			if got, want := dep.IsValid(), tc.state != stateInvalid; got != want {
				t.Errorf("IsValid() = %v, want %v", got, want)
			}
			if got, want := dep.IsPartial(), tc.state == statePartial; got != want {
				t.Errorf("IsPartial() = %v, want %v", got, want)
			}
			if got, want := dep.IsComplete(), tc.state == stateComplete; got != want {
				t.Errorf("IsComplete() = %v, want %v", got, want)
			}

			// IsValid is the union of the two stricter states, which must be
			// mutually exclusive.
			if dep.IsPartial() && dep.IsComplete() {
				t.Error("IsPartial() and IsComplete() must not both be true")
			}
			if dep.IsValid() != (dep.IsPartial() || dep.IsComplete()) {
				t.Error("IsValid() must equal IsPartial() || IsComplete()")
			}
		})
	}
}

func TestParseDependency(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Dependency
	}{
		{"full spec", "@preview/cetz:0.3.0", Dependency{Namespace: "preview", Name: "cetz", Version: "0.3.0"}},
		{"no version", "@preview/cetz", Dependency{Namespace: "preview", Name: "cetz"}},
		{"namespace with dash", "@my-ns/foo:v1.2.3", Dependency{Namespace: "my-ns", Name: "foo", Version: "v1.2.3"}},
		{"empty input", "", Dependency{}},
		{"no slash", "preview", Dependency{}},
		{"only namespace", "@preview", Dependency{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseDependency(tt.in); got != tt.want {
				t.Errorf("ParseDependency(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestDependencyString(t *testing.T) {
	tests := []struct {
		name string
		dep  Dependency
		want string
	}{
		{"full", Dependency{Namespace: "preview", Name: "cetz", Version: "0.3.0"}, "@preview/cetz:0.3.0"},
		{"no version", Dependency{Namespace: "preview", Name: "cetz"}, "@preview/cetz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dep.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDependencyRelPath(t *testing.T) {
	full := Dependency{Namespace: "preview", Name: "cetz", Version: "0.3.0"}
	if want := filepath.Join("preview", "cetz", "0.3.0"); full.RelPath() != want {
		t.Errorf("RelPath() = %q, want %q", full.RelPath(), want)
	}

	partial := Dependency{Namespace: "preview", Name: "cetz"}
	if got := partial.RelPath(); got != "" {
		t.Errorf("RelPath() for partial spec = %q, want empty", got)
	}
}
