package fs

import (
	"log/slog"
	"path/filepath"

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
	// Under the concrete type, for anything that wants api/fs itself...
	a.AddService(fm)
	// ...and under the core's interface, which is what App.FileSystem()
	// resolves. The adapter exists because api/fs.Disk returns fsys.FS and
	// the core declares its own identically-shaped Disk: Go needs the method
	// signatures to match exactly for interface satisfaction, so a return
	// type of fsys.FS does not satisfy one of app.Disk even though every
	// value it returns does. One method, and it keeps the AWS and Google
	// Cloud SDKs out of every package that imports api/app.
	app.RegisterIfAbsent[app.FileSystem](a, disks{fm})

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

// disks adapts api/fs to the app.FileSystem seam. See Provide.
type disks struct {
	fm *fs.FileSystem
}

func (d disks) Disk(name ...string) (app.Disk, error) {
	return d.fm.Disk(name...)
}

// Compile-time proof that the adapter still satisfies the seam it is
// registered under. The signature is the whole subtlety here, so a
// declaration is the right place to catch it going wrong.
var _ app.FileSystem = disks{}

func Get(a app.App) *fs.FileSystem {
	return app.Get[*fs.FileSystem](a)
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

	// Absolute, which is the entire point of logging it.
	//
	// GetUrl answers the configured path joined with the request, so for a
	// disk configured as "./storage/app/public" it answers
	// "storage/app/public" — the value from the configuration file, which is
	// the one thing the reader already knows. What is worth printing is where
	// that resolved to in this process, because the failure being diagnosed
	// is a relative path and a working directory nobody chose.
	root := ""
	if located, locateErr := disk.GetUrl("."); locateErr == nil {
		root = located
		if absolute, absErr := filepath.Abs(located); absErr == nil {
			root = absolute
		}
	}
	slog.Info("Serving stored files",
		"prefix", prefix, "disk", fss.PublicDisk, "driver", disk.Driver(),
		"root", root, "dirs", fss.Public, "entire_disk", fss.PublicEntireDisk)
}
