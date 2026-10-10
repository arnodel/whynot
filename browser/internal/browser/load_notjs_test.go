//go:build !js

package browser

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewRegistryRoot checks that with a root, local documents are only
// loaded from beneath it, and that without one, any local document is.
func TestNewRegistryRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	inside := writeTempMD(t, filepath.Join(dir, "docs"), "in.md", "# In")
	outside := writeTempMD(t, dir, "out.md", "# Out")
	root, err := os.OpenRoot(filepath.Join(dir, "docs"))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	confined := NewRegistry(root)
	if _, err := LoadDocument(confined, inside); err != nil {
		t.Errorf("document beneath the root: err = %v, want nil", err)
	}
	if _, err := LoadDocument(confined, outside); err == nil {
		t.Error("document outside the root: err = nil, want an error")
	}
	if _, err := LoadDocument(NewRegistry(nil), outside); err != nil {
		t.Errorf("without a root: err = %v, want nil", err)
	}
}
