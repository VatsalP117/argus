package localsecret

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCreatesAndReusesPrivateSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets", "author-hash-salt")

	first, err := Ensure(path)
	if err != nil {
		t.Fatalf("create secret: %v", err)
	}
	second, err := Ensure(path)
	if err != nil {
		t.Fatalf("reuse secret: %v", err)
	}
	if first == "" || first != second {
		t.Fatal("expected a stable non-empty secret")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat secret: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 0600, got %o", info.Mode().Perm())
	}
}
