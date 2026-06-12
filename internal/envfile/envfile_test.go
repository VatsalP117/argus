package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSetsUnsetVariablesOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=bar\nexport BAZ=qux\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	t.Setenv("BAZ", "keep")
	if err := Load(path); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got := os.Getenv("FOO"); got != "bar" {
		t.Fatalf("FOO = %q, want bar", got)
	}
	if got := os.Getenv("BAZ"); got != "keep" {
		t.Fatalf("BAZ = %q, want keep", got)
	}
}

func TestLoadIgnoresMissingFiles(t *testing.T) {
	if err := Load(filepath.Join(t.TempDir(), ".missing")); err != nil {
		t.Fatalf("Load returned error for missing file: %v", err)
	}
}
