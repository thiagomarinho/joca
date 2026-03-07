package version_test

import (
	"testing"

	"github.com/thiagomarinho/joca/version"
)

func TestVersionDefaults(t *testing.T) {
	if version.Version == "" {
		t.Error("Version must not be empty")
	}
	if version.Commit == "" {
		t.Error("Commit must not be empty")
	}
}
