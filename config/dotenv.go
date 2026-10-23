package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// DotEnvFile is the name of the environment file loaded at startup.
const DotEnvFile = ".env"

// DotEnvTestFile is loaded instead of DotEnvFile when the process is a test
// binary and the file exists.
//
// **Instead of, not in addition to.** That is the entire point: a suite that
// cannot read .env cannot reach the credentials in it. Layering the two would
// leave every variable .env.test forgot to override pointing wherever
// development points — which, on the day this was written, was a production
// database in another continent.
//
// A project opts in by creating the file. Without one nothing changes, which
// matters because this package's init runs in every test binary that imports
// it, including those of libraries that have no .env at all and never wanted
// one.
const DotEnvTestFile = ".env.test"

// init loads the environment file, searching upward from the working
// directory.
//
// This replaces godotenv's autoload, which looks in the working directory and
// nowhere else. That is correct when a binary is started from the project
// root, and wrong in the one place it matters most: `go test ./internal/...`
// runs each package with the working directory set to that package, so .env
// is never found and every config.MustEnv call silently falls back to its
// default. A test then runs against an application configured differently
// from the one that ships — an unset APP_KEY means no JWT secret, which means
// no token, which means an authentication test fails for a reason that has
// nothing to do with authentication.
//
// Searching upward also matches what people expect from the equivalent tool
// in other frameworks, where .env belongs to the project rather than to
// whichever directory a process happened to start in.
//
// Existing environment variables always win: godotenv.Load never overwrites,
// so a value exported by the shell, a CI runner or a container beats the file,
// which is the precedence anyone deploying this relies on.
func init() {
	if path, ok := findEnvFile(); ok {
		// A malformed file is worth knowing about, but not worth taking the
		// process down for at init time, before logging is configured. The
		// variables simply stay unset and the defaults apply.
		_ = godotenv.Load(path)
	}
}

// findEnvFile returns the environment file this process should load.
//
// Under a test binary a .env.test anywhere up the tree wins outright, and the
// .env beside it is not read at all.
func findEnvFile() (string, bool) {
	if UnderTest() {
		if path, ok := findUpward(DotEnvTestFile); ok {
			return path, true
		}
	}
	return findUpward(DotEnvFile)
}

// UnderTest reports whether this process is a Go test binary.
//
// It reads the binary's own arguments rather than APP_ENV, and that is a
// correction rather than a preference. A test harness is handed the real
// configuration, so app.env is whatever .env says — which made it useless as
// a signal, as /log found when it tried exactly that. The go tool names test
// binaries "<pkg>.test" and passes -test.* flags to them, neither of which
// anybody has to remember to set.
func UnderTest() bool {
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.") {
			return true
		}
	}
	return strings.HasSuffix(filepath.Base(os.Args[0]), ".test")
}

// findUpward walks up from the working directory looking for name, stopping at
// the filesystem root.
//
// It does not stop at go.mod. A project may keep its environment file beside
// go.mod, but a test binary's working directory is a package below it and some
// projects keep the file a level higher still, so the search continues to the
// root rather than guessing where the boundary is.
func findUpward(name string) (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// findDotEnv finds .env specifically, ignoring .env.test.
//
// Kept because the tests for the upward search are written against it and the
// search is the part worth testing on its own.
func findDotEnv() (string, bool) { return findUpward(DotEnvFile) }
