package fs

import (
	"errors"
	"fmt"
	"github.com/lemmego/api/config"
	"github.com/lemmego/fsys"
	"os"
)

type FilesystemManager struct {
	disks  map[string]fsys.FS
	config *config.Config
}

func NewFilesystemManager(c *config.Config) *FilesystemManager {
	return &FilesystemManager{disks: map[string]fsys.FS{}, config: c}
}

func (fm *FilesystemManager) Get(diskName ...string) (fsys.FS, error) {
	var name string

	if len(diskName) > 0 {
		name = diskName[0]
	} else {
		name = os.Getenv("FILESYSTEM_DISK")
	}

	if name == "" {
		return nil, errors.New("default disk could not be found")
	}

	if _, ok := fm.disks[name]; !ok {
		fm.disks[name] = Resolve(name, fm.config)
	}

	return fm.disks[name], nil
}

func Resolve(name string, c *config.Config) fsys.FS {
	if conf, ok := c.Get("filesystems.disks").(config.M)[name].(config.M); ok {
		switch conf["driver"] {
		case "local":
			return fsys.NewLocalStorage(c.Get(fmt.Sprintf("filesystems.disks.%s.path", name)).(string))
		case "s3":
			fs, err := fsys.NewS3Storage(
				c.Get(fmt.Sprintf("filesystems.disks.%s.bucket", name)).(string),
				c.Get(fmt.Sprintf("filesystems.disks.%s.region", name)).(string),
				c.Get(fmt.Sprintf("filesystems.disks.%s.key", name)).(string),
				c.Get(fmt.Sprintf("filesystems.disks.%s.secret", name)).(string),
				c.Get(fmt.Sprintf("filesystems.disks.%s.endpoint", name)).(string),
			)
			if err != nil {
				panic(err)
			}
			return fs
		}
	}

	return fsys.NewLocalStorage(c.Get("filesystems.disks.local.path").(string))
}
