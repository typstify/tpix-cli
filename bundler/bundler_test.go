package bundler

import "testing"

func TestShouldExclude(t *testing.T) {
	creator := NewPackageCreator(nil)

	tests := []struct {
		name     string
		path     string
		patterns []string
		expected bool
	}{
		// Simple file glob patterns
		{
			name:     "glob *.txt matches txt file",
			path:     "readme.txt",
			patterns: []string{"*.txt"},
			expected: true,
		},
		{
			name:     "glob *.txt does not match md file",
			path:     "readme.md",
			patterns: []string{"*.txt"},
			expected: false,
		},

		// Recursive glob **
		{
			name:     "recursive ** matches nested file",
			path:     "src/components/button.typ",
			patterns: []string{"src/**"},
			expected: true,
		},
		{
			name:     "recursive ** matches in subdirectory",
			path:     "src/lib/utils.typ",
			patterns: []string{"src/**/*.typ"},
			expected: true,
		},
		{
			name:     "recursive ** does not match outside pattern",
			path:     "lib/utils.typ",
			patterns: []string{"src/**"},
			expected: false,
		},

		// Directory exclusion (trailing /)
		{
			name:     "directory tests/ excludes contents",
			path:     "tests/helpers.typ",
			patterns: []string{"tests/"},
			expected: true,
		},
		{
			name:     "directory tests/ excludes nested",
			path:     "tests/unit/integration.typ",
			patterns: []string{"tests/"},
			expected: true,
		},
		{
			name:     "directory exclude does not match similar prefix",
			path:     "testsuite/main.typ",
			patterns: []string{"tests/"},
			expected: false,
		},

		// Double-star directory exclusion
		{
			name:     "node_modules/** excludes nested",
			path:     "node_modules/pkg/index.js",
			patterns: []string{"node_modules/**"},
			expected: true,
		},
		{
			name:     "node_modules/** matches directory itself with doublestar",
			path:     "node_modules",
			patterns: []string{"node_modules/**"},
			expected: true, // doublestar ** matches zero or more segments
		},

		// Hidden files (dot files)
		{
			name:     "exact dot file match",
			path:     ".gitignore",
			patterns: []string{".gitignore"},
			expected: true,
		},
		{
			name:     "dot glob matches dotfile",
			path:     ".DS_Store",
			patterns: []string{".*"},
			expected: true,
		},

		// Multiple patterns (OR logic)
		{
			name:     "matches any pattern",
			path:     "debug.log",
			patterns: []string{"*.txt", "*.log"},
			expected: true,
		},
		{
			name:     "matches second pattern",
			path:     "temp",
			patterns: []string{"*.tmp", "temp"},
			expected: true,
		},
		{
			name:     "no pattern matches",
			path:     "main.typ",
			patterns: []string{"*.txt", "tests/"},
			expected: false,
		},

		// Exact file/directory names
		{
			name:     "exact directory name with trailing slash",
			path:     "dist/bundle.js",
			patterns: []string{"dist/"},
			expected: true,
		},
		{
			name:     "exact file name in subdir",
			path:     "typst.toml",
			patterns: []string{"typst.toml"},
			expected: true,
		},

		// Path with leading ./
		{
			name:     "pattern with leading ./",
			path:     "src/main.typ",
			patterns: []string{"./src/*"},
			expected: true,
		},

		// Wildcard in middle of path
		{
			name:     "wildcard in middle",
			path:     "lib/v1/utils.typ",
			patterns: []string{"lib/*/utils.typ"},
			expected: true,
		},

		// Edge cases
		{
			name:     "empty patterns",
			path:     "anything.typ",
			patterns: []string{},
			expected: false,
		},
		{
			name:     "no match with complex path",
			path:     "src/lib/core.typ",
			patterns: []string{"lib/**"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := creator.shouldExclude(tt.path, tt.patterns)
			if result != tt.expected {
				t.Errorf("shouldExclude(%q, %v) = %v, want %v", tt.path, tt.patterns, result, tt.expected)
			}
		})
	}
}