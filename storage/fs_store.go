package storage

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/typstify/tpix-cli/deps"
	"github.com/typstify/tpix-cli/utils"
)

var (
	_ PackageStore = (*FsPackageStore)(nil)
	_ ContentStore = (*FsPackageStore)(nil)
)

type FsPackageStore struct {
	cacheDir string
	rwLock   sync.RWMutex
	// per package view lock
	viewLocks map[pkgLockKey]*sync.RWMutex
	mapMu     sync.Mutex
}

type pkgLockKey struct {
	namespace string
	name      string
}

func NewFsPackageStore(cacheDir string) (*FsPackageStore, error) {
	if cacheDir == "" {
		return nil, errors.New("empty package cache dir")
	}

	cacheDir, err := filepath.Abs(cacheDir)
	if err != nil {
		return nil, err
	}

	if stat, err := os.Stat(cacheDir); err != nil {
		return nil, err
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", cacheDir)
	}

	return &FsPackageStore{
		cacheDir:  cacheDir,
		viewLocks: make(map[pkgLockKey]*sync.RWMutex),
	}, nil

}

func (f *FsPackageStore) lockFor(pkg deps.Dependency) *sync.RWMutex {
	key := pkgLockKey{namespace: pkg.Namespace, name: pkg.Name}

	f.mapMu.Lock()
	defer f.mapMu.Unlock()

	lock, ok := f.viewLocks[key]
	if !ok {
		lock = &sync.RWMutex{}
		f.viewLocks[key] = lock
	}

	return lock
}

// Add implements [PackageStore].
func (f *FsPackageStore) Add(pkg deps.Dependency, archive io.Reader) error {
	// take the view lock first
	viewLock := f.lockFor(pkg)
	viewLock.Lock()
	defer viewLock.Unlock()

	// use read lock so add new packages is not serialized in the store.
	f.rwLock.RLock()
	defer f.rwLock.RUnlock()

	destDir, err := f.Path(pkg)
	if err != nil {
		return err
	}

	pkgDir := filepath.Join(f.cacheDir, pkg.Namespace, pkg.Name)
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(pkgDir, ".pkg-staging-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	if err := utils.ExtractTarGzFromReader(archive, staging); err != nil {
		return err
	}
	_ = os.RemoveAll(destDir) // remove existing first.

	// atomic rename operation to prevents partial write of package files.
	return os.Rename(staging, destDir)

}

// Has implements [PackageStore].
func (f *FsPackageStore) Has(pkg deps.Dependency) (bool, error) {
	destDir, err := f.Path(pkg)
	if err != nil {
		return false, err
	}

	// take the view lock first
	viewLock := f.lockFor(pkg)
	viewLock.RLock()
	defer viewLock.RUnlock()

	if stat, err := os.Stat(destDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}

		return false, err

	} else if !stat.IsDir() {
		return false, fmt.Errorf("%s is not a directory", destDir)
	}

	return true, nil
}

// List implements [PackageStore].
func (f *FsPackageStore) List(filter func(spec deps.Dependency) bool) ([]deps.Dependency, error) {
	// use a write lock to obtain a exclusive snapshot of the store.
	f.rwLock.Lock()
	defer f.rwLock.Unlock()

	entries, err := os.ReadDir(f.cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache directory: %w", err)
	}

	allPkgs := make([]deps.Dependency, 0)

	for _, namespace := range entries {
		if !namespace.IsDir() {
			continue
		}
		namespacePath := filepath.Join(f.cacheDir, namespace.Name())
		pkgs, err := os.ReadDir(namespacePath)
		if err != nil {
			continue
		}
		for _, pkg := range pkgs {
			if !pkg.IsDir() {
				continue
			}
			pkgPath := filepath.Join(namespacePath, pkg.Name())
			versions, err := os.ReadDir(pkgPath)
			if err != nil {
				return nil, err
			}
			for _, version := range versions {
				if !version.IsDir() {
					continue
				}

				spec := deps.Dependency{Namespace: namespace.Name(), Name: pkg.Name(), Version: version.Name()}
				if filter != nil && !filter(spec) {
					continue
				}

				allPkgs = append(allPkgs, spec)

			}
		}
	}

	return allPkgs, nil
}

// Path implements [PackageStore].
func (f *FsPackageStore) Path(pkg deps.Dependency) (string, error) {
	if !pkg.IsComplete() {
		return "", fmt.Errorf("invalid pkg spec: %s", pkg)
	}

	return filepath.Join(f.cacheDir, pkg.Namespace, pkg.Name, pkg.Version), nil
}

// Remove implements [PackageStore].
func (f *FsPackageStore) Remove(pkg deps.Dependency) error {
	if !pkg.IsValid() {
		return fmt.Errorf("invalid pkg spec: %s", pkg)
	}

	// take the view lock first
	viewLock := f.lockFor(pkg)
	viewLock.Lock()
	defer viewLock.Unlock()

	f.rwLock.Lock()
	defer f.rwLock.Unlock()

	if pkg.IsPartial() {
		// Remove every version of the package.
		return os.RemoveAll(filepath.Join(f.cacheDir, pkg.Namespace, pkg.Name))
	}

	dir, err := f.Path(pkg)
	if err != nil {
		return err
	}

	return os.RemoveAll(dir)
}

// View implements [ContentStore].
func (f *FsPackageStore) View(pkg deps.Dependency) (PackageView, error) {
	dir, err := f.Path(pkg)
	if err != nil {
		return nil, err
	}

	// take the view lock first
	viewLock := f.lockFor(pkg)
	viewLock.RLock()

	view, err := newFsPackageView(dir, viewLock.RUnlock)
	if err != nil {
		viewLock.RUnlock()
		return nil, err
	}

	return view, err
}

// A package view basd on os file system.
// Callers must call Close to release the internal root, and package view lock,
// otherwise Add and Remove of the same package in the package store will be
// blocked.
type FsPackageView struct {
	path    string
	root    *os.Root
	release func()
	once    sync.Once
}

func newFsPackageView(pkgPath string, release func()) (PackageView, error) {
	root, err := os.OpenRoot(pkgPath)
	if err != nil {
		return nil, err
	}

	return &FsPackageView{path: pkgPath, root: root, release: release}, nil

}

func (v *FsPackageView) Open(name string) (fs.File, error) {
	if v.root == nil {
		return nil, fmt.Errorf("package view not initialized: %s", v.path)
	}

	return v.root.Open(name)
}

func (v *FsPackageView) Close() error {
	var err error

	v.once.Do(func() {
		if v.root != nil {
			err = v.root.Close()
		}

		// release package view lock
		if v.release != nil {
			v.release()
		}
	})

	return err
}
