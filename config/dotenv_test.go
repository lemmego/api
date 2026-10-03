package config

import (
	"os"
	"path/filepath"
	"testing"
)

// godotenv's autoload looks in the working directory and nowhere else. That
// is wrong in the place it matters most: `go test ./internal/...` runs each
// package with its own working directory, so .env is never found and every
// MustEnv call silently falls back to a default — and a test then exercises
// an application configured differently from the one that ships.
func TestFindDotEnvSearchesUpward(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "internal", "routes")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(root, DotEnvFile)
	if err := os.WriteFile(envPath, []byte("PROBE=found\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(nested)

	found, ok := findDotEnv()
	if !ok {
		t.Fatal("findDotEnv did not find .env two directories up")
	}
	// The temp dir may be a symlink (/var -> /private/var on macOS), so
	// compare resolved paths rather than the strings.
	gotReal, _ := filepath.EvalSymlinks(found)
	wantReal, _ := filepath.EvalSymlinks(envPath)
	if gotReal != wantReal {
		t.Errorf("found %q, want %q", gotReal, wantReal)
	}
}

// The nearest .env wins, so a package-level fixture can still override the
// project's.
func TestFindDotEnvPrefersTheNearest(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "internal")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, DotEnvFile), []byte("PROBE=outer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(nested, DotEnvFile)
	if err := os.WriteFile(inner, []byte("PROBE=inner\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(nested)

	found, ok := findDotEnv()
	if !ok {
		t.Fatal("findDotEnv found nothing")
	}
	gotReal, _ := filepath.EvalSymlinks(found)
	wantReal, _ := filepath.EvalSymlinks(inner)
	if gotReal != wantReal {
		t.Errorf("found %q, want the nearer %q", gotReal, wantReal)
	}
}

func TestFindDotEnvReportsAbsence(t *testing.T) {
	// A temp dir has no .env anywhere above it except, conceivably, the
	// filesystem root — which would be a very odd machine. Walking to the
	// root and returning false is the behaviour under test: it must not loop.
	t.Chdir(t.TempDir())
	if path, ok := findDotEnv(); ok && filepath.Dir(path) != "/" {
		t.Errorf("findDotEnv claimed %q", path)
	}
}

// A directory named .env is not an environment file, and must not be treated
// as one.
func TestFindDotEnvIgnoresADirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, DotEnvFile), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	if path, ok := findDotEnv(); ok && filepath.Dir(path) == root {
		t.Errorf("findDotEnv returned the directory %q", path)
	}
}
