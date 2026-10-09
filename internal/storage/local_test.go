package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func dirMode(t *testing.T, dir string) os.FileMode {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat %s: %v", dir, err)
	}
	return info.Mode().Perm()
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(entries) != 0 {
		t.Fatalf("%s holds %d entries after the probe, want none", dir, len(entries))
	}
}

func TestEnsureWritableDirCreatesMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "var", "lib", "odyssey", "storage")

	if err := EnsureWritableDir(dir); err != nil {
		t.Fatalf("EnsureWritableDir(%q) error = %v", dir, err)
	}
	if mode := dirMode(t, dir); mode&^0o750 != 0 {
		t.Fatalf("created mode = %o, want no bits beyond 0750", mode)
	}
	assertEmptyDir(t, dir)
}

func TestEnsureWritableDirFailsOnReadOnlyDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	dir := filepath.Join(t.TempDir(), "readonly")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o550); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	if err := EnsureWritableDir(dir); err == nil {
		t.Fatalf("EnsureWritableDir(%q) succeeded on a read-only directory", dir)
	}
	if mode := dirMode(t, dir); mode != 0o550 {
		t.Fatalf("mode after EnsureWritableDir = %o, want unchanged 0550", mode)
	}
}

func TestEnsureWritableDirNeverChangesModeBits(t *testing.T) {
	for _, mode := range []os.FileMode{0o700, 0o750, 0o770, 0o755} {
		t.Run(mode.String(), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "existing")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, mode); err != nil {
				t.Fatal(err)
			}

			if err := EnsureWritableDir(dir); err != nil {
				t.Fatalf("EnsureWritableDir(%q) error = %v", dir, err)
			}
			if got := dirMode(t, dir); got != mode {
				t.Fatalf("mode after EnsureWritableDir = %o, want unchanged %o", got, mode)
			}
			assertEmptyDir(t, dir)
		})
	}
}

func TestEnsureWritableDirRejectsEmptyAndFilePaths(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plain-file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"", "   ", file} {
		if err := EnsureWritableDir(dir); err == nil {
			t.Fatalf("EnsureWritableDir(%q) succeeded, want an error", dir)
		}
	}
}
