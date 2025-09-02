package fs

import (
	"fmt"
	"github.com/lemmego/api/app"
	"github.com/lemmego/api/fs"
	"reflect"
)

type Provider struct {
	fm *fs.FileSystem
}

func (fss *Provider) Provide(a app.App) error {
	fmt.Println("Registering FS")
	fm := fs.NewFileSystem()
	fss.fm = fm
	a.AddService(fm)
	return nil
}

func Get(a app.App) *fs.FileSystem {
	return a.Service(reflect.TypeOf(&fs.FileSystem{})).(*fs.FileSystem)
}
