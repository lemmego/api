package providers

import (
	"github.com/lemmego/api/app"
	"github.com/lemmego/api/di"
	"github.com/lemmego/api/fs"
)

func init() {
	app.RegisterService(func(a app.App) error {
		fm := fs.NewFilesystemManager()
		return di.For[*fs.FilesystemManager](a.Container()).
			AsSingleton().
			UseInstance(fm)
	})
}
