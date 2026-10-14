package app

import (
	"io"
	"mime/multipart"
	"os"
)

// Disk is one storage backend, and FileSystem resolves disks by name.
//
// These are declared here rather than imported, and the reason is measured
// rather than stylistic. The App interface used to return *fs.FileSystem, and
// Context.disk returned fsys.FS, so the core imported lemmego/fsys — which
// serves local, S3 and GCS from a single package, and therefore links the AWS
// SDK and seven cloud.google.com modules. `go list -deps ./api/app` was 773
// packages, 81 of them cloud SDKs, plus nine OpenTelemetry modules and three
// SQL drivers, for a framework whose core formats HTTP responses.
//
// That is the cost every consumer of this package paid, whether or not it ever
// wrote a file, and it is the same mistake lemmego/log already recorded: with
// its provider inside the module, importing the core dragged in 289 modules
// for a package that formats text. media/deps_test.go goes further and
// *refuses* to depend on fsys by name, for exactly this reason — so the core
// was doing the thing one of its own modules declines to do.
//
// Every method here takes and returns standard-library types only, which is
// what makes the split possible: fsys.FS satisfies Disk structurally, so
// nothing has to be converted, wrapped per call, or kept in sync beyond the
// method set. The drivers stay in api/fs, which is reached through
// api/providers/fs — weight in the provider, contract in the core.
//
// api/app/deps_test.go asserts the result, because an interface declared in
// the wrong package is precisely how this would come back.
type Disk interface {
	// Driver returns the name of the current driver.
	Driver() string

	// Read a file from storage.
	Read(path string) (io.ReadCloser, error)

	// Write a file to storage.
	Write(path string, contents []byte) error

	// Delete a file from storage.
	Delete(path string) error

	// Exists reports whether a file exists in storage.
	Exists(path string) (bool, error)

	// Rename a file in storage.
	Rename(oldPath, newPath string) error

	// Copy a file in storage.
	Copy(sourcePath, destinationPath string) error

	// CreateDirectory creates a directory if it does not already exist.
	CreateDirectory(path string) error

	// GetUrl returns a URL for a file. Local storage may return a path and
	// cloud storage a URL, so this is not meaningful on every driver.
	GetUrl(path string) (string, error)

	// Open opens a file.
	Open(path string) (*os.File, error)

	// Upload writes an uploaded file to the driver.
	Upload(file multipart.File, header *multipart.FileHeader, dir string) (*os.File, error)
}

// FileSystem resolves a Disk by name, or the configured default when no name
// is given.
//
// An implementation is registered under this interface by api/providers/fs.
// An application that never registered it gets a nil FileSystem rather than a
// panic, which is what the nil checks in Context's file methods have always
// been written for — they were unreachable while the accessor used Get.
type FileSystem interface {
	Disk(name ...string) (Disk, error)
}
