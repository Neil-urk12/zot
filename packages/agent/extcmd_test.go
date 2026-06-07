package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExtInstallDotSource verifies that `zot ext install .` derives the
// extension name from the resolved directory name rather than collapsing
// to the extensions/ parent directory (the false "already exists" bug).
func TestExtInstallDotSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZOT_HOME", home)

	// Pre-create extensions/ to mimic a normal first run.
	if err := os.MkdirAll(filepath.Join(home, "extensions"), 0o755); err != nil {
		t.Fatal(err)
	}

	srcParent := t.TempDir()
	src := filepath.Join(srcParent, "kagi")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "extension.json"), []byte(`{"name":"kagi"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	if err := os.Chdir(src); err != nil {
		t.Fatal(err)
	}

	if err := extInstall([]string{"."}); err != nil {
		t.Fatalf("install with '.' failed: %v", err)
	}

	out := filepath.Join(home, "extensions", "kagi")
	if _, err := os.Stat(filepath.Join(out, "extension.json")); err != nil {
		t.Fatalf("expected installed extension at %s: %v", out, err)
	}
}

// TestExtInstallRejectsParentName guards against deriving a name of ".."
// from a source that resolves to a filesystem root edge case. A normal
// directory always yields a real basename, so this just ensures the
// guard logic does not crash for well-formed input.
func TestExtInstallNamedDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZOT_HOME", home)

	src := filepath.Join(t.TempDir(), "myext")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "extension.json"), []byte(`{"name":"myext"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := extInstall([]string{src}); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "extensions", "myext", "extension.json")); err != nil {
		t.Fatalf("expected installed extension: %v", err)
	}
}
