package fs

import (
	"log/slog"

	"github.com/lemmego/api/app"
	"github.com/lemmego/api/fs"
)

type Provider struct {
	fm *fs.FileSystem

	// Public names the directories inside the default disk that may be
	// served over HTTP, mounted under PublicPrefix.
	//
	// Empty — the default — serves nothing. An application that has not said
	// which of its uploads are public does not get the rest published because
	// a provider guessed: the same disk usually holds the logs, the sessions
	// and, in a SQLite project, the database.
	Public []string

	// PublicPrefix is where Public is mounted. Defaults to "/storage/", which
	// is what a path stored as "/storage/avatars/x.png" expects.
	PublicPrefix string

	// PublicDisk is the disk Public is read from. Empty means the default
	// disk, which is where Context.Upload writes.
	PublicDisk string
}

func (fss *Provider) Provide(a app.App) error {
	slog.Debug("Registering FS")
	fm := fs.NewFileSystem()
	fss.fm = fm
	a.AddService(fm)

	if len(fss.Public) > 0 {
		prefix := fss.PublicPrefix
		if prefix == "" {
			prefix = "/storage/"
		}
		// A subtree pattern, so one route covers every file beneath it. It is
		// more specific than the framework's "/{path...}" catch-all, so it
		// wins whichever order they are registered in — and until something
		// claims it, a request for a stored file is answered by the 404 page,
		// which is how a stored image renders as a broken one.
		a.Router().Handle("GET "+prefix, fm.Serve(fs.ServeOptions{
			Prefix: prefix,
			Dirs:   fss.Public,
			Disk:   fss.PublicDisk,
		}))
		slog.Debug("Serving stored files", "prefix", prefix, "dirs", fss.Public)
	}

	return nil
}

func Get(a app.App) *fs.FileSystem {
	return app.Get[*fs.FileSystem](a)
	// return a.Service(reflect.TypeOf(&fs.FileSystem{})).(*fs.FileSystem)
}
