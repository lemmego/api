package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
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

// A test binary prefers .env.test.
//
// The whole point of the file is that a suite configures itself, so if this
// resolution is wrong nothing else about it matters.
func TestFindEnvFilePrefersTheTestFileUnderTest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, DotEnvFile), []byte("WHICH=dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, DotEnvTestFile), []byte("WHICH=test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	// This is a test binary, so UnderTest is true without arranging anything.
	if !UnderTest() {
		t.Fatal("UnderTest is false inside a test binary")
	}
	found, ok := findEnvFile()
	if !ok {
		t.Fatal("findEnvFile found nothing")
	}
	if filepath.Base(found) != DotEnvTestFile {
		t.Errorf("found %q, want %s", found, DotEnvTestFile)
	}
}

// **.env is not read when .env.test exists.**
//
// Instead of, not in addition to, which is the guarantee the file exists to
// give. If the two layered, every variable .env.test did not think to
// override would still point wherever development points — and a suite could
// reach a production database without anybody doing anything wrong.
func TestTheDevelopmentFileIsNotLoadedAlongsideTheTestFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, DotEnvFile),
		[]byte("ONLY_IN_DEV=yes\nSHARED=dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, DotEnvTestFile),
		[]byte("SHARED=test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	found, ok := findEnvFile()
	if !ok {
		t.Fatal("findEnvFile found nothing")
	}
	if filepath.Base(found) != DotEnvTestFile {
		t.Fatalf("found %q, want %s", found, DotEnvTestFile)
	}

	// Load exactly what init loads, and read it back. ONLY_IN_DEV exists in
	// no other file, so its presence would mean .env had been read too.
	os.Unsetenv("ONLY_IN_DEV")
	os.Unsetenv("SHARED")
	t.Cleanup(func() {
		os.Unsetenv("ONLY_IN_DEV")
		os.Unsetenv("SHARED")
	})
	if err := godotenv.Load(found); err != nil {
		t.Fatalf("loading %s: %v", found, err)
	}

	if got := os.Getenv("SHARED"); got != "test" {
		t.Errorf("SHARED = %q, want %q", got, "test")
	}
	if got := os.Getenv("ONLY_IN_DEV"); got != "" {
		t.Errorf("a variable defined only in %s reached the test environment: ONLY_IN_DEV=%q",
			DotEnvFile, got)
	}
}

// Without a .env.test, nothing changes.
//
// This package's init runs in every test binary that imports it, including
// those of libraries with no .env at all. Requiring the file would break all
// of them, so a project opts in by creating one.
func TestWithoutATestFileTheOrdinaryOneIsUsed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, DotEnvFile), []byte("WHICH=dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	found, ok := findEnvFile()
	if !ok {
		t.Fatal("findEnvFile found nothing")
	}
	if filepath.Base(found) != DotEnvFile {
		t.Errorf("found %q, want %s", found, DotEnvFile)
	}
}
