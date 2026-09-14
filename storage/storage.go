// package storage defines interfaces for the storage of typst package cache,
// and retrieving of packages from the package cache.
package storage

import (
	"io"
	"io/fs"

	"github.com/typstify/tpix-cli/deps"
)

// PackageStore defines the main storage APIs for the typst package cache.
type PackageStore interface {
	// Has checks if pkg exists in the cache, returning error if something
	// goes wrong during the check.
	Has(pkg deps.Dependency) (bool, error)

	// Add adds a package to the storage, with package namespace, name and version
	// specified by pkg. The content should be a tar.gz archive reader.
	Add(pkg deps.Dependency, archive io.Reader) error

	// List returns all versioned packages inside the package store. If a filter
	// is provided, it is used to filter out the result, otherwise all results are
	// returned. If the filter returns true, the package is returned, otherwise
	// the package is omitted.
	List(filter func(spec deps.Dependency) bool) ([]deps.Dependency, error)

	// Remove a package from the store. When a specific version is provided, that
	// version is removed, otherwise the entire package versions are removed.
	Remove(pkg deps.Dependency) error

	// Path returns the underlying storage path for pkg.
	Path(pkg deps.Dependency) (string, error)
}

// PackageView defines a sandboxed, read-only view of one package.
// Paths are slash-seperated and relative to the package root,
// e,g. "typst.toml", "templates/main.typ".
type PackageView interface {
	fs.FS
	// Closer is used to release the underlying root handle.
	io.Closer
}

// ContentStore defines APIs to get a package view from the package store.
type ContentStore interface {
	View(pkg deps.Dependency) (PackageView, error)
}

type Store interface {
	PackageStore
	ContentStore
}