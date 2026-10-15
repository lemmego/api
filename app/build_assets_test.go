package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lemmego/api/config"
)

// The built frontend was served by nothing.
//
// Vite writes the bundle to public/build, and lemmego/inertia emits
// manifest-based tags under its BuildDir default of "/build/" — so a
// production page asks for /build/assets/app-HASH.js. The mux carried
// /static/ and /public/ and neither matches that, so every asset fell through
// to the catch-all and answered 404.
//
// It passed unnoticed because development takes a different branch entirely:
// with public/hot present the tags point at the Vite dev server, so the page
// is correct under `lemmego dev` and arrives in production with
// server-rendered markup, no stylesheet and no hydration. The SSR guard looks
// for server-rendered markup, which was there, so nothing reported it either.
func TestTheBuiltFrontendIsServedAtBuild(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "public", "build", "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	const bundle = "console.log('hydrated')"
	if err := os.WriteFile(filepath.Join(assets, "app-BqS5Qndi.js"), []byte(bundle), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	handler := Configure(WithConfig(config.M{"app": config.M{"name": "test"}})).Handler()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/build/assets/app-BqS5Qndi.js", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("GET /build/assets/app-BqS5Qndi.js = %d, want 200 — the built "+
			"bundle is unreachable, so a production page renders unstyled and "+
			"never hydrates", response.Code)
	}
	if got := response.Body.String(); got != bundle {
		t.Errorf("body = %q, want %q", got, bundle)
	}
}

// A project with no frontend build must not change behaviour: the directory is
// absent and the handler answers 404, which is what the catch-all already did.
func TestAMissingBuildDirectoryStillAnswers404(t *testing.T) {
	t.Chdir(t.TempDir())

	handler := Configure(WithConfig(config.M{"app": config.M{"name": "test"}})).Handler()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/build/assets/nothing.js", nil))

	if response.Code != http.StatusNotFound {
		t.Errorf("GET a missing asset = %d, want 404", response.Code)
	}
}
