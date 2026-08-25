package deps

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var importRegex = regexp.MustCompile(`#import\s+"@([^/]+)/([^:]+):([^"]+)"`)

// ExtractFromSource scans a single .typ file's content for package imports.
func ExtractFromSource(content []byte) []Dependency {
	seen := make(map[string]struct{})
	var deps []Dependency

	lines := bytes.Split(content, []byte("\n"))
	inBlockComment := false

	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)

		if inBlockComment {
			if idx := bytes.Index(trimmed, []byte("*/")); idx >= 0 {
				inBlockComment = false
				trimmed = trimmed[idx+2:]
			} else {
				continue
			}
		}

		if idx := bytes.Index(trimmed, []byte("/*")); idx >= 0 {
			if closeIdx := bytes.Index(trimmed[idx+2:], []byte("*/")); closeIdx >= 0 {
				before := trimmed[:idx]
				after := trimmed[idx+2+closeIdx+2:]
				trimmed = append(before, after...)
			} else {
				trimmed = trimmed[:idx]
				inBlockComment = true
			}
		}

		if bytes.HasPrefix(trimmed, []byte("//")) {
			continue
		}

		if idx := bytes.Index(trimmed, []byte("//")); idx >= 0 {
			trimmed = trimmed[:idx]
		}

		matches := importRegex.FindAllSubmatch(trimmed, -1)
		for _, match := range matches {
			if len(match) == 4 {
				dep := Dependency{
					Namespace: string(match[1]),
					Name:      string(match[2]),
					Version:   string(match[3]),
				}
				if _, ok := seen[dep.String()]; !ok {
					seen[dep.String()] = struct{}{}
					deps = append(deps, dep)
				}
			}
		}
	}

	return deps
}

// ExtractFromDirectory walks a local directory, scanning all .typ files for imports.
func ExtractFromDirectory(dirPath string) ([]Dependency, error) {
	seen := make(map[string]struct{})
	var deps []Dependency

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".typ") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		for _, dep := range ExtractFromSource(content) {
			if _, ok := seen[dep.String()]; !ok {
				seen[dep.String()] = struct{}{}
				deps = append(deps, dep)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return deps, nil
}
