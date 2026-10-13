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

	// EntireDisk serves everything on the disk instead of the directories in
	// Dirs, and is only ever correct for a disk whose root holds nothing
	// private.
	//
	// That is what a dedicated public disk is for — root it at
	// storage/app/public, keep the logs, the sessions and the database
	// outside it, and "what is public" becomes a fact about the filesystem
	// rather than a list somebody has to remember to keep short. Dirs is the
	// other way round: it is for serving part of a disk that also holds
	// things it must not serve, which works but puts the security property
	// in Go code that a later edit can widen.
	//
	// It is a separate field rather than "Dirs empty means everything"
	// because the dangerous reading has to be the one nobody reaches by
	// accident: an empty Dirs serves nothing.
	EntireDisk bool

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
// It answers 404 for everything it refuses, including a path outside Dirs or
// off the disk.
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
	entire := opts.EntireDisk

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set before anything can return, so it is on the refusals too.
		w.Header().Set("X-Content-Type-Options", "nosniff")

		name, ok := storedPath(prefix, dirs, entire, r.URL.Path)
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
func storedPath(prefix string, dirs []string, entire bool, urlPath string) (string, bool) {
	if len(dirs) == 0 && !entire {
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

	if entire {
		// Every directory under the root is reachable, and a file directly in
		// it too — which is the difference from the Dirs case, where a path
		// with no directory segment cannot be matched at all.
		//
		// Containment rests on two independent layers, and it is worth being
		// precise about which does what: the cleaning above removes every
		// ".." before the path is used, and fsys's own resolvePath refuses a
		// path that lands outside the disk root or resolves there through a
		// symlink. Removing the cleaning does not open an escape here —
		// proven by mutating it, where only the Dirs test failed — because
		// the disk layer still refuses. The cleaning is what makes the Dirs
		// allowlist meaningful, since without it "posts/../logs/app.log"
		// would be tested as being in "posts".
		return cleaned, true
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
