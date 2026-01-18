// Package fs provides file system abstraction for the Lemmego framework.
//
// It offers a unified interface for file operations across different storage
// backends including local filesystem, AWS S3, Google Cloud Storage, and others.
// The package manages multiple disk configurations and provides automatic
// resolution of storage drivers based on configuration.
package fs

import (
	"errors"
	"fmt"

	"github.com/lemmego/api/config"
	"github.com/lemmego/fsys"
)

// FileSystem manages multiple file system disks and provides access to different
// storage backends through a unified interface. It handles disk resolution,
// caching, and configuration-based storage driver selection.
type FileSystem struct {
	disks map[string]fsys.FS // Cache of initialized file system instances
}

// NewFileSystem creates a new FileSystem manager with an empty disk cache.
func NewFileSystem() *FileSystem {
	return &FileSystem{disks: map[string]fsys.FS{}}
}

func (fm *FileSystem) Disk(diskName ...string) (fsys.FS, error) {
	var name string

	if len(diskName) > 0 {
		name = diskName[0]
	} else {
		name = config.MustEnv("FILESYSTEM_DISK", "local")
	}

	if name == "" {
		return nil, errors.New("default disk could not be found")
	}

	if _, ok := fm.disks[name]; !ok {
		fm.disks[name] = resolve(name)
	}

	return fm.disks[name], nil
}

func resolve(name string) fsys.FS {
	if conf, ok := config.Get("filesystems.disks").(config.M)[name].(config.M); ok {
		switch conf["driver"] {
		case "local":
			return fsys.NewLocalStorage(config.Get(fmt.Sprintf("filesystems.disks.%s.path", name)).(string))
		case "s3":
			fs, err := fsys.NewS3Storage(
				config.Get(fmt.Sprintf("filesystems.disks.%s.bucket", name)).(string),
				config.Get(fmt.Sprintf("filesystems.disks.%s.region", name)).(string),
				config.Get(fmt.Sprintf("filesystems.disks.%s.key", name)).(string),
				config.Get(fmt.Sprintf("filesystems.disks.%s.secret", name)).(string),
				config.Get(fmt.Sprintf("filesystems.disks.%s.endpoint", name)).(string),
			)
			if err != nil {
				panic(err)
			}
			return fs
		}
	}

	return fsys.NewLocalStorage(config.Get("filesystems.disks.local.path").(string))
}
