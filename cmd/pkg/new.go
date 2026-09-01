package pkg

import (
	"errors"
	"fmt"
)

// Create a empty package using builtin template manifest. Returning the dir of
// of package, and a optional error.
func CreatePkg(pkgDir string, namespace string, name string, isTemplate bool, username string, email string) (string, error) {
	author := username
	if username != "" && email != "" {
		author = fmt.Sprintf("%s <%s>", username, email)
	}

	if namespace == "" {
		return "", errors.New("missing namespace name")
	}

	if name == "" {
		return "", errors.New("missing package name")
	}

	return createSkeleton(pkgDir, namespace, name, isTemplate, author)
}
