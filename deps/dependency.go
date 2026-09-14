package deps

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Dependency represents a parsed Typst package import.
type Dependency struct {
	Namespace string
	Name      string
	Version   string
}

// Helper functions

// ParseDependency parses a package spec in the format @namespace/name:version
// Returns namespace, name, and version (version may be empty)
func ParseDependency(pkgName string) Dependency {
	// Remove leading @ and split on /
	s := strings.TrimPrefix(pkgName, "@")
	parts := strings.SplitN(s, "/", 2)
	if len(parts) < 2 {
		return Dependency{}
	}

	spec := Dependency{Namespace: parts[0]}

	// Split name and version on :
	nameVer := strings.SplitN(parts[1], ":", 2)
	spec.Name = nameVer[0]

	if len(nameVer) > 1 {
		spec.Version = nameVer[1]
	}

	return spec
}

// state classifies how completely a Dependency identifies a package.
type state uint8

const (
	// stateInvalid means the namespace or name is missing, so the spec does
	// not identify a package.
	stateInvalid state = iota
	// statePartial means the spec identifies a package but does not pin a
	// version.
	statePartial
	// stateComplete means the spec fully identifies one package version.
	stateComplete
)

// State reports how completely s identifies a package.
func (s Dependency) state() state {
	switch {
	case s.Namespace == "" || s.Name == "":
		return stateInvalid
	case s.Version == "":
		return statePartial
	default:
		return stateComplete
	}
}

// IsValid reports whether s identifies at least a package (partial or complete).
func (s Dependency) IsValid() bool { return s.state() != stateInvalid }

// IsPartial reports whether s identifies a package without a pinned version.
func (s Dependency) IsPartial() bool { return s.state() == statePartial }

// IsComplete reports whether s fully identifies one package version.
func (s Dependency) IsComplete() bool { return s.state() == stateComplete }

// RelPath returns the file system path of the package relative to the cache dir.
// It returns an empty path unless s is complete.
func (s Dependency) RelPath() string {
	if !s.IsComplete() {
		return ""
	}

	return filepath.Join(s.Namespace, s.Name, s.Version)
}

var _ fmt.Stringer = Dependency{}

// String returns the normalized string form of the package.
func (s Dependency) String() string {
	if !s.IsComplete() {
		return fmt.Sprintf("@%s/%s", s.Namespace, s.Name)
	}

	return fmt.Sprintf("@%s/%s:%s", s.Namespace, s.Name, s.Version)

}
