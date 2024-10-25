package app

import "github.com/lemmego/api/fs"

type FilesystemProvider struct {
	*ServiceProvider
}

func (provider *FilesystemProvider) Register(a AppManager) {
	fm := fs.NewFilesystemManager(a.Config())
	a.AddService(fm)
}

func (provider *FilesystemProvider) Boot(a AppManager) {
	//
}
