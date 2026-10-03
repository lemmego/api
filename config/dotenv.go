package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// DotEnvFile is the name of the environment file loaded at startup.
const DotEnvFile = ".env"

// init loads .env, searching upward from the working directory.
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
	if path, ok := findDotEnv(); ok {
		// A malformed .env is worth knowing about, but not worth taking the
		// process down for at init time, before logging is configured. The
		// variables simply stay unset and the defaults apply.
		_ = godotenv.Load(path)
	}
}

// findDotEnv walks up from the working directory looking for .env, stopping at
// the filesystem root.
//
// It does not stop at go.mod. A project may keep .env beside go.mod, but a
// test binary's working directory is a package below it and some projects keep
// the file a level higher still, so the search continues to the root rather
// than guessing where the boundary is.
func findDotEnv() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(dir, DotEnvFile)
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
