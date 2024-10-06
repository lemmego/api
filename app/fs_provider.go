package app

import (
	"fmt"
	"github.com/lemmego/api/config"
	"github.com/lemmego/api/fsys"
)

type FSServiceProvider struct {
	*BaseServiceProvider
}

func (provider *FSServiceProvider) Register(app *App) {
	app.Bind((*fsys.FS)(nil), func() fsys.FS {
		var fs fsys.FS
		fmt.Println(config.Get[map[any]any]("storage"))
		switch config.MustEnv("FILESYSTEM_DISK", "local") {
		case "local":
			fs = fsys.NewLocalStorage("")
		case "s3":
			if s3, err := fsys.NewS3Storage(
				config.Get[string]("storage.s3.bucket"),
				config.Get[string]("storage.s3.region"),
				config.Get[string]("storage.s3.key"),
				config.Get[string]("storage.s3.secret"),
				config.Get[string]("storage.s3.endpoint"),
			); err != nil {
				panic(fmt.Sprintf("Failed to initialize S3 storage: %s", err))
			} else {
				fs = s3
			}
		}
		return fs
	})
}

func (provider *FSServiceProvider) Boot() {
	//
}
