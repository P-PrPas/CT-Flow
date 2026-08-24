// Package testsupport holds helpers shared by tests across packages.
//
// It exists for one reason: several packages read shared fixtures --
// tests/testdata/*.json, db/schema.sql, models.json -- and reaching them with
// "../../.." breaks the moment a package moves between directories. It broke
// exactly that way when the Go tree was reorganised into transport/core/infra/
// platform layers, which is a silly thing to debug twice.
//
// Only ever imported from _test.go files.
package testsupport

import (
	"os"
	"path/filepath"
)

// BackendFile resolves a path relative to the backend module root (the
// directory holding go.mod), regardless of how deeply nested the caller is.
func BackendFile(rel string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, rel), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist // walked to the filesystem root without finding go.mod
		}
		dir = parent
	}
}

// MustBackendFile is BackendFile for callers that would only turn the error
// into a fatal anyway.
func MustBackendFile(rel string) string {
	p, err := BackendFile(rel)
	if err != nil {
		panic("testsupport: cannot locate the backend module root: " + err.Error())
	}
	return p
}
