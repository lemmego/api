package fsys

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"

	"github.com/lemmego/api/config"
)

// FS defines the methods that any storage system must implement.
type FS interface {
	// Driver returns the name of the current driver
	Driver() string

	// Read a file from storage.
	Read(path string) (io.ReadCloser, error)

	// Write a file to storage.
	Write(path string, contents []byte) error

	// Delete a file from storage.
	Delete(path string) error

	// Exists checks if a file exists in storage.
	Exists(path string) (bool, error)

	// Rename a file in storage.
	Rename(oldPath, newPath string) error

	// Copy a file in storage.
	Copy(sourcePath, destinationPath string) error

	// CreateDirectory creates a new directory if doesn't already exist for the given path
	CreateDirectory(path string) error

	// GetUrl gets the URL for a file in storage (optional).
	// This method may not be applicable to all storage systems.
	// For example, local storage may return a file path, while cloud storage may return a URL.
	GetUrl(path string) (string, error)

	// Open opens a file
	Open(path string) (*os.File, error)

	// Upload uploads a file to the implemented driver
	Upload(file multipart.File, header *multipart.FileHeader, dir string) (*os.File, error)
}

type FilesystemManager struct {
	disks  map[string]FS
	config *config.Config
}

func NewFilesystemManager(c *config.Config) *FilesystemManager {
	return &FilesystemManager{disks: map[string]FS{}}
}

func (fm *FilesystemManager) Disk(name string) FS {
	if _, ok := fm.disks[name]; !ok {
		fm.disks[name] = Resolve(name, fm.config)
	}
	return fm.disks[name]
}

func Resolve(name string, c *config.Config) FS {
	if conf, ok := c.Get("filesystems.disks").(config.M)[name].(config.M); ok {
		switch conf["driver"] {
		case "local":
			return NewLocalStorage(c.Get(fmt.Sprintf("filesystems.disks.%s.path", name)).(string))
		case "s3":
			fs, err := NewS3Storage(
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

	return NewLocalStorage(c.Get("filesystems.disks.local.path").(string))
}
