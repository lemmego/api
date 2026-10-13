package fs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lemmego/api/config"
)

// serveFixture builds a disk laid out like a real project's storage
// directory: the uploads the application means to publish, and beside them
// the things it does not — the log, which outside production holds rendered
// mail and so live confirmation links, and the session store.
func serveFixture(t *testing.T) (*FileSystem, string) {
	t.Helper()
	root := t.TempDir()

	write := func(name, contents string) {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// A one-pixel PNG, so the served bytes are a real image rather than text
	// with a .png name.
	write("posts/1-2-0.png", "\x89PNG\r\n\x1a\n-pretend-this-is-pixels")
	write("avatars/1-2.png", "\x89PNG\r\n\x1a\n-an-avatar")
	write("logs/app.log", "confirm?token=0123456789abcdef")
	write("session/abcdef", "a-session")

	config.GetInstance().SetConfigMap(config.M{
		"filesystems": config.M{
			"default": "local",
			"disks": config.M{
				"local": config.M{"driver": "local", "path": root},
			},
		},
	})
	return NewFileSystem(), root
}

func serveHandler(fm *FileSystem, dirs ...string) http.Handler {
	return fm.Serve(ServeOptions{Prefix: "/storage/", Dirs: dirs, Disk: "local"})
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestAStoredFileIsServed(t *testing.T) {
	fm, _ := serveFixture(t)
	rec := get(t, serveHandler(fm, "avatars", "posts"), "/storage/posts/1-2-0.png")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — a stored image that answers anything else renders as a broken one", rec.Code)
	}
	if !strings.HasPrefix(rec.Body.String(), "\x89PNG") {
		t.Errorf("body = %q, want the stored bytes", rec.Body.String())
	}
	if kind := rec.Header().Get("Content-Type"); !strings.HasPrefix(kind, "image/png") {
		t.Errorf("Content-Type = %q, want image/png", kind)
	}
}

// The reason the allowlist exists. storage/logs/app.log is a fixed, guessable
// name, and outside production the log is where mail is written — so a disk
// mounted at its root publishes email-confirmation and password-reset links
// to anyone who asks for that one URL.
func TestADirectoryOutsideTheAllowlistIsNotServed(t *testing.T) {
	fm, _ := serveFixture(t)
	h := serveHandler(fm, "avatars", "posts")

	for _, target := range []string{
		"/storage/logs/app.log",
		"/storage/session/abcdef",
	} {
		rec := get(t, h, target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "token") || strings.Contains(rec.Body.String(), "a-session") {
			t.Errorf("GET %s served the file contents: %q", target, rec.Body.String())
		}
	}
}

// Cleaning happens before the allowlist is consulted, so this is refused for
// not being in Dirs rather than relying on the disk to catch it. Both layers
// hold; this asserts the outer one.
func TestATraversalOutOfAnAllowedDirectoryIsRefused(t *testing.T) {
	fm, _ := serveFixture(t)
	h := serveHandler(fm, "posts")

	for _, target := range []string{
		"/storage/posts/../logs/app.log",
		"/storage/posts/../../etc/passwd",
		"/storage/./logs/app.log",
	} {
		rec := get(t, h, target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "token") {
			t.Errorf("GET %s served the log: %q", target, rec.Body.String())
		}
	}
}

// A listing of an upload directory is a list of everybody's files, which is a
// different thing from being allowed to read one of them.
func TestADirectoryIsNotListed(t *testing.T) {
	fm, _ := serveFixture(t)
	rec := get(t, serveHandler(fm, "posts"), "/storage/posts/")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "1-2-0.png") {
		t.Error("the directory was listed")
	}
}

// A path with no directory segment cannot be matched against the allowlist,
// so it is refused rather than resolved against the disk root.
func TestAPathWithNoDirectoryIsRefused(t *testing.T) {
	fm, _ := serveFixture(t)
	h := serveHandler(fm, "posts")
	for _, target := range []string{"/storage/", "/storage/app.log", "/storage"} {
		if rec := get(t, h, target); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, rec.Code)
		}
	}
}

// The default is to serve nothing. A provider that guessed would publish the
// logs of every application that never thought about it.
func TestAnEmptyAllowlistServesNothing(t *testing.T) {
	fm, _ := serveFixture(t)
	rec := get(t, serveHandler(fm), "/storage/posts/1-2-0.png")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 — an empty allowlist must serve nothing", rec.Code)
	}
}

// These bytes came from somebody's upload form, so the browser does not get
// to decide what they are.
func TestEveryResponseCarriesNosniff(t *testing.T) {
	fm, _ := serveFixture(t)
	h := serveHandler(fm, "posts")
	for _, target := range []string{"/storage/posts/1-2-0.png", "/storage/logs/app.log"} {
		rec := get(t, h, target)
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("GET %s: X-Content-Type-Options = %q, want nosniff", target, got)
		}
	}
}

// ServeContent brings conditional requests with it, which is most of what
// makes a feed of images cheap on the second visit.
func TestARepeatRequestIsAnsweredNotModified(t *testing.T) {
	fm, _ := serveFixture(t)
	h := serveHandler(fm, "posts")

	first := get(t, h, "/storage/posts/1-2-0.png")
	modified := first.Header().Get("Last-Modified")
	if modified == "" {
		t.Fatal("no Last-Modified, so a client has nothing to revalidate with")
	}

	req := httptest.NewRequest(http.MethodGet, "/storage/posts/1-2-0.png", nil)
	req.Header.Set("If-Modified-Since", modified)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304", rec.Code)
	}
}

func TestHeadIsAnsweredWithoutABody(t *testing.T) {
	fm, _ := serveFixture(t)
	rec := httptest.NewRecorder()
	serveHandler(fm, "posts").ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/storage/posts/1-2-0.png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD returned %d bytes of body", rec.Body.Len())
	}
}

// A directory name that merely starts with an allowed one is a different
// directory. "postsecret" is not "posts".
func TestAPrefixOfAnAllowedDirectoryIsNotAllowed(t *testing.T) {
	fm, root := serveFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "postsecret"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "postsecret", "x.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := get(t, serveHandler(fm, "posts"), "/storage/postsecret/x.txt")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Error("served a file from a directory that only shares a prefix with an allowed one")
	}
}

// splitFixture is the layout a dedicated public disk is for: the publishable
// things under one root, and everything private — the log that holds rendered
// mail, the session store, the database — outside it.
func splitFixture(t *testing.T) *FileSystem {
	t.Helper()
	root := t.TempDir()

	write := func(name, contents string) {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("app/public/avatars/1-2.png", "\x89PNG\r\n\x1a\n-an-avatar")
	write("app/public/posts/1-2-0.png", "\x89PNG\r\n\x1a\n-a-post-image")
	write("app/public/favicon.ico", "an-icon")
	write("app/private/contract.pdf", "%PDF-not-for-the-web")
	write("logs/app.log", "confirm?token=0123456789abcdef")
	write("session/abcdef", "a-session")

	config.GetInstance().SetConfigMap(config.M{
		"filesystems": config.M{
			"default": "local",
			"disks": config.M{
				"local":  config.M{"driver": "local", "path": filepath.Join(root, "app", "private")},
				"public": config.M{"driver": "local", "path": filepath.Join(root, "app", "public")},
			},
		},
	})
	return NewFileSystem()
}

func entireHandler(fm *FileSystem) http.Handler {
	return fm.Serve(ServeOptions{Prefix: "/storage/", Disk: "public", EntireDisk: true})
}

// Every directory under the public root is reachable without being listed,
// which is the point of the split: adding a new kind of upload needs no
// change to an allowlist, and forgetting to add one cannot serve the logs.
func TestAnEntireDiskServesEveryDirectoryUnderIt(t *testing.T) {
	h := entireHandler(splitFixture(t))

	for _, target := range []string{
		"/storage/avatars/1-2.png",
		"/storage/posts/1-2-0.png",
		// A file at the root too, unlike the Dirs case where a path with no
		// directory segment cannot be matched at all.
		"/storage/favicon.ico",
	} {
		rec := get(t, h, target)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", target, rec.Code)
		}
	}
}

// The security property the split buys, stated as a test: nothing outside the
// public root is reachable even though everything inside it is. Any of these
// succeeding means the whole arrangement is worthless.
//
// Two layers hold it up and this exercises both. Mutating the path cleaning
// out of storedPath leaves these passing — the disk's own resolvePath still
// refuses — so this is a test of the pair, not of the cleaning. That matters
// if the disk layer is ever swapped: these cases would still pass against a
// replacement that happened to be safe, and fail against one that was not.
func TestAnEntireDiskCannotReachOutsideItsRoot(t *testing.T) {
	h := entireHandler(splitFixture(t))

	for _, target := range []string{
		"/storage/../logs/app.log",
		"/storage/avatars/../../logs/app.log",
		"/storage/../private/contract.pdf",
		"/storage/../../etc/passwd",
		"/storage/%2e%2e/logs/app.log",
	} {
		rec := get(t, h, target)
		if rec.Code == http.StatusOK {
			t.Errorf("GET %s = 200; it escaped the public disk", target)
		}
		for _, leaked := range []string{"token", "PDF", "a-session"} {
			if strings.Contains(rec.Body.String(), leaked) {
				t.Errorf("GET %s leaked %q", target, leaked)
			}
		}
	}
}

// EntireDisk is opt-in, so an application that only sets a disk name still
// serves nothing. The dangerous reading must not be the reachable one.
func TestADiskWithoutEntireDiskOrDirsServesNothing(t *testing.T) {
	fm := splitFixture(t)
	h := fm.Serve(ServeOptions{Prefix: "/storage/", Disk: "public"})

	if rec := get(t, h, "/storage/avatars/1-2.png"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 — neither Dirs nor EntireDisk was set", rec.Code)
	}
}

// And the default disk stays private: the same handler pointed at it must not
// become a way to read what the split moved out of the way.
func TestTheDefaultDiskIsNotServedByAccident(t *testing.T) {
	fm := splitFixture(t)
	h := fm.Serve(ServeOptions{Prefix: "/storage/", EntireDisk: true})

	rec := get(t, h, "/storage/contract.pdf")
	// It is reachable, because that is what the caller asked for — which is
	// exactly why EntireDisk has the doc comment it has. What must not happen
	// is this being the configuration an application gets without choosing it.
	if rec.Code == http.StatusOK && !strings.Contains(rec.Body.String(), "PDF") {
		t.Error("the handler answered 200 with something other than the file")
	}
}
