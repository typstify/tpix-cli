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

// RelPath returns the file system path of the package relative to the cache dir.
func (s Dependency) RelPath() string {
	if !s.Partial() {
		return ""
	}

	return filepath.Join(s.Namespace, s.Name, s.Version)
}

var _ fmt.Stringer = Dependency{}

// String returns the normalized string form of the package.
func (s Dependency) String() string {
	if !s.Partial() {
		return fmt.Sprintf("@%s/%s", s.Namespace, s.Name)
	}

	return fmt.Sprintf("@%s/%s:%s", s.Namespace, s.Name, s.Version)

}

// Partial checks if s has all fields or not.
func (s Dependency) Partial() bool {
	return s.Namespace != "" && s.Name != "" && s.Version != ""
}
