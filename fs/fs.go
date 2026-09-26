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
	"sync"

	"github.com/lemmego/api/config"
	"github.com/lemmego/fsys"
)

// FileSystem manages multiple file system disks and provides access to different
// storage backends through a unified interface. It handles disk resolution,
// caching, and configuration-based storage driver selection.
type FileSystem struct {
	disks map[string]fsys.FS // Cache of initialized file system instances
	mu    sync.RWMutex
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

	fm.mu.RLock()
	disk, ok := fm.disks[name]
	fm.mu.RUnlock()
	if ok {
		return disk, nil
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()
	if fm.disks == nil {
		fm.disks = make(map[string]fsys.FS)
	}
	if disk, ok := fm.disks[name]; ok {
		return disk, nil
	}
	disk, err := resolve(name)
	if err != nil {
		return nil, err
	}
	fm.disks[name] = disk
	return disk, nil
}

// defaultLocalPath is where a local disk stores files when the configuration
// does not say. A project with no filesystems configuration still gets a
// working local disk rather than a panic.
const defaultLocalPath = "./storage"

// resolve builds the named disk.
//
// Every configuration read here is guarded. It previously asserted the disk
// map and five S3 settings without checking, so an application with no
// filesystems section — or an S3 disk missing a key — took the process down on
// the first upload. A disk that cannot be built is now an error the caller can
// report.
func resolve(name string) (fsys.FS, error) {
	disks, _ := config.Get("filesystems.disks").(config.M)
	conf, _ := disks[name].(config.M)

	// An unconfigured disk falls back to local storage. Returning an error
	// instead would break the common case of a project that never configured
	// filesystems and never uploads anything.
	driver, _ := conf["driver"].(string)
	if driver == "" {
		driver = "local"
	}

	setting := func(key string) string {
		value, _ := conf[key].(string)
		return value
	}

	switch driver {
	case "local":
		path := setting("path")
		if path == "" {
			path = defaultLocalPath
		}
		return fsys.NewLocalStorage(path), nil

	case "s3":
		store, err := fsys.NewS3Storage(
			setting("bucket"),
			setting("region"),
			setting("key"),
			setting("secret"),
			setting("endpoint"),
		)
		if err != nil {
			return nil, fmt.Errorf("fs: building the %q disk: %w", name, err)
		}
		return store, nil

	default:
		return nil, fmt.Errorf("fs: disk %q has unsupported driver %q (supported: local, s3)", name, driver)
	}
}
