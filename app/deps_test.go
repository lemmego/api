package app_test

import (
	"os/exec"
	"strings"
	"testing"
)

// The core must not charge every consumer for two cloud SDKs.
//
// It did. The App interface returned *fs.FileSystem and Context.disk returned
// fsys.FS, so this package imported lemmego/fsys — which serves local, S3 and
// GCS from one package, and therefore links the AWS SDK and seven
// cloud.google.com modules. `go list -deps ./api/app` was 773 packages, 81 of
// them cloud SDKs, with nine OpenTelemetry modules and three SQL drivers
// behind them. Every package that imports api/app paid for that, uploads or
// no uploads: pay's root package was 74 modules and seo's 75, against
// broadcast's 1 and log's 1.
//
// It is now an interface declared here (see disk.go) with the drivers reached
// through api/providers/fs. The measurement that matters is this one, not the
// intention, because an interface declared in the wrong package is exactly
// how it would come back — and nothing about the code would look wrong.
//
// lemmego/log is the precedent this repo already recorded: with its provider
// inside the module, a consumer importing only the core inherited 289 modules
// for a package that formats text. notify and media each carry the same guard
// for the same reason. The core was the one place that had the problem and no
// guard.
func TestTheCoreDoesNotLinkACloudSDK(t *testing.T) {
	deps := dependencies(t)

	// Substrings rather than exact paths: a vendored or renamed SDK is the
	// same mistake, and an SDK adds packages under its own prefix freely.
	forbidden := []string{
		"github.com/aws/aws-sdk-go",
		"cloud.google.com/",
		"go.opentelemetry.io/",
		"google.golang.org/grpc",
		"github.com/lemmego/fsys",
	}

	for _, dep := range deps {
		for _, bad := range forbidden {
			if strings.Contains(dep, bad) {
				t.Errorf("api/app links %s — the storage drivers belong in "+
					"api/fs, reached through api/providers/fs", dep)
			}
		}
	}
}

// A ceiling, not a target. The point is that somebody adding an import that
// doubles the core's dependency graph has to come here and say so, which is
// the conversation the number exists to start.
//
// The split above took it from 773 to 300. The headroom is deliberate: most
// of what remains is lemmego/migration blank-importing lib/pq,
// go-sql-driver/mysql and modernc.org/sqlite so that `migrate` can run, which
// is a real finding and a separate change — moving those registrations into a
// migration/drivers subpackage would take another ~90 packages off this.
func TestTheCoreStaysSmall(t *testing.T) {
	const ceiling = 400

	if count := len(dependencies(t)); count > ceiling {
		t.Errorf("api/app reaches %d packages, over the %d ceiling; if the "+
			"import that did it is genuinely required, raise the ceiling in "+
			"the same commit and say why", count, ceiling)
	}
}

func dependencies(t *testing.T) []string {
	t.Helper()

	// GOFLAGS is cleared of -mod=vendor and the like by running in this
	// directory with the default environment; go list reports the build list
	// the test binary itself was built from.
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	var deps []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			deps = append(deps, line)
		}
	}
	if len(deps) == 0 {
		t.Fatal("go list -deps returned nothing")
	}
	return deps
}
