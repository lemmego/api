package fs

import (
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/lemmego/fsys"
)

// ServeOptions says which stored files may be fetched over HTTP.
type ServeOptions struct {
	// Prefix is the URL prefix the handler is mounted at, such as
	// "/storage/". It is stripped before the rest of the path is resolved
	// against the disk.
	Prefix string

	// Dirs are the directories inside the disk that may be served. Nothing
	// else is reachable.
	//
	// There is deliberately no "serve the whole disk" option, and this is the
	// reason: a disk's root holds whatever the application put there. In a
	// scaffolded project that is storage/logs, storage/session and
	// storage/database.sqlite sitting beside the uploads — and the log is
	// where mail is written outside production, which is to say it holds live
	// email-confirmation and password-reset links. A root mount publishes all
	// of that at a guessable URL. An allowlist cannot be widened by accident.
	Dirs []string

	// Disk is the disk to read from. Empty means the default disk.
	Disk string
}

// Serve returns a handler that serves uploaded files from a disk.
//
// The framework has had the writing half of this for a long time —
// Context.Upload and Context.StoreUpload put a file on a disk — and no
// reading half at all, so an application stored an avatar and then had
// nothing to serve it with. The failure is quiet in the usual way: the file
// is on disk, the row holds the right path, and the img element asks for a
// URL that nothing answers. The browser shows a broken image and the server
// log shows a 404 among the 404s.
//
// It answers 404 for everything it refuses, including a path outside Dirs.
// Whether a file exists somewhere the caller may not read is not information
// worth handing out, and a 403 hands it out.
//
// A note on content types, because the allowlist does not cover them: this
// serves whatever bytes were stored under a name chosen by the application,
// with the type derived from the extension. If the application lets somebody
// upload "payload.html" and stores it under that extension, this will serve
// it as text/html from the application's own origin, which is a stored
// cross-site scripting vector and no amount of path allowlisting helps.
// X-Content-Type-Options: nosniff is set on every response, which closes the
// sniffing half of that; the other half is the upload rules —
// Validator.FileTypes sniffs the content rather than trusting the filename,
// and a handler should name the stored file from what was sniffed rather than
// from what the client called it.
func (fm *FileSystem) Serve(opts ServeOptions) http.Handler {
	prefix := opts.Prefix
	if prefix == "" {
		prefix = "/"
	}
	dirs := slices.Clone(opts.Dirs)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set before anything can return, so it is on the refusals too.
		w.Header().Set("X-Content-Type-Options", "nosniff")

		name, ok := storedPath(prefix, dirs, r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		var (
			disk fsys.FS
			err  error
		)
		if opts.Disk == "" {
			disk, err = fm.Disk()
		} else {
			disk, err = fm.Disk(opts.Disk)
		}
		if err != nil {
			slog.Error("fs: serving a stored file", "path", name, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if disk.Driver() == fsys.DRIVER_LOCAL {
			serveLocal(w, r, disk, name)
			return
		}
		serveStream(w, r, disk, name)
	})
}

// serveLocal hands the file to ServeContent, which brings range requests and
// conditional GETs with it. Worth the separate path: a feed of images is
// mostly re-requests, and a 304 is the difference between a page that feels
// instant and one that re-downloads every picture.
func serveLocal(w http.ResponseWriter, r *http.Request, disk fsys.FS, name string) {
	file, err := disk.Open(name)
	if err != nil {
		// Open resolves the path against the disk root and refuses anything
		// outside it, so this covers both "no such file" and a traversal that
		// survived storedPath.
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		// No directory listings. A listing of an upload directory is a list
		// of everybody's files, which is a different thing from being allowed
		// to read one of them.
		http.NotFound(w, r)
		return
	}

	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

// serveStream copies the bytes for a disk that is not a local directory.
//
// Read is the only accessor every driver implements: Open is local-only in
// practice (the memory driver refuses it outright), and GetUrl answers a
// filesystem path for a local disk and "mem://..." for the memory one, so
// redirecting to it is not something that can be done in general. A remote
// disk fronted by its own CDN should be referenced by that URL rather than
// through this handler; this path exists so that swapping the disk does not
// break the application.
func serveStream(w http.ResponseWriter, r *http.Request, disk fsys.FS, name string) {
	body, err := disk.Read(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer body.Close()

	if kind := mime.TypeByExtension(path.Ext(name)); kind != "" {
		w.Header().Set("Content-Type", kind)
	}
	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, body); err != nil {
		slog.Debug("fs: streaming a stored file ended early", "path", name, "error", err)
	}
}

// storedPath turns a request path into a path on the disk, or refuses it.
//
// The cleaning happens before the allowlist is consulted, which is the whole
// point of doing it here: a request for "posts/../logs/app.log" is tested as
// "logs/app.log" and refused for not being in Dirs, rather than being passed
// to the disk as something that looks allowed.
func storedPath(prefix string, dirs []string, urlPath string) (string, bool) {
	if len(dirs) == 0 {
		return "", false
	}

	rest := strings.TrimPrefix(urlPath, prefix)
	if rest == urlPath && prefix != "/" {
		return "", false
	}

	// Clean against a leading slash so that no "..", however many, can climb
	// above the root; then drop the slash to get a disk-relative path.
	cleaned := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(rest, "/")), "/")
	if cleaned == "" || cleaned == "." {
		return "", false
	}

	dir, file, found := strings.Cut(cleaned, "/")
	if !found || file == "" {
		return "", false
	}
	if !slices.Contains(dirs, dir) {
		return "", false
	}
	return cleaned, true
}
