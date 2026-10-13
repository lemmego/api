package fs

import (
	"log/slog"

	"github.com/lemmego/api/app"
	"github.com/lemmego/api/fs"
	"github.com/lemmego/fsys"
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

	// PublicEntireDisk serves all of PublicDisk rather than the directories
	// in Public, and is correct only for a disk rooted somewhere that holds
	// nothing private — a disk at storage/app/public, with the logs, the
	// sessions and the database outside it. Prefer it to listing directories:
	// it puts the boundary in the filesystem instead of in a slice that a
	// later edit can widen.
	PublicEntireDisk bool
}

func (fss *Provider) Provide(a app.App) error {
	slog.Debug("Registering FS")
	fm := fs.NewFileSystem()
	fss.fm = fm
	a.AddService(fm)

	if len(fss.Public) > 0 || fss.PublicEntireDisk {
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
			Prefix:     prefix,
			Dirs:       fss.Public,
			Disk:       fss.PublicDisk,
			EntireDisk: fss.PublicEntireDisk,
		}))
		// At info, with the resolved root, because the commonest way for this
		// to be wrong is a relative disk path and a process whose working
		// directory is not the project: the files are written and served from
		// wherever the process happens to be standing, and the symptom is a
		// 404 among the 404s. A single binary under systemd with no
		// WorkingDirectory set is exactly that case.
		fss.logServedRoot(fm, prefix)
	}

	return nil
}

func Get(a app.App) *fs.FileSystem {
	return app.Get[*fs.FileSystem](a)
	// return a.Service(reflect.TypeOf(&fs.FileSystem{})).(*fs.FileSystem)
}

// logServedRoot says where stored files are being served from.
func (fss *Provider) logServedRoot(fm *fs.FileSystem, prefix string) {
	var (
		disk fsys.FS
		err  error
	)
	if fss.PublicDisk == "" {
		disk, err = fm.Disk()
	} else {
		disk, err = fm.Disk(fss.PublicDisk)
	}
	if err != nil {
		slog.Error("fs: the public disk could not be opened, so no stored file will be served",
			"disk", fss.PublicDisk, "error", err)
		return
	}

	root := ""
	if located, locateErr := disk.GetUrl("."); locateErr == nil {
		root = located
	}
	slog.Info("Serving stored files",
		"prefix", prefix, "disk", fss.PublicDisk, "driver", disk.Driver(),
		"root", root, "dirs", fss.Public, "entire_disk", fss.PublicEntireDisk)
}
