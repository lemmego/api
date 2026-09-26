package fs

import (
	"testing"

	"github.com/lemmego/api/config"
)

// A project with no filesystems configuration must still get a working local
// disk. Every read in resolve was previously an unguarded assertion, and the
// very first one — config.Get("filesystems.disks").(config.M)[name] — is a
// single-value assertion on a nil, so an absent section panicked before any
// fallback could run.
func TestResolveWithoutAnyFilesystemsConfig(t *testing.T) {
	config.Set("filesystems", nil)

	disk, err := resolve("local")
	if err != nil {
		t.Fatalf("resolve() with no config error = %v", err)
	}
	if disk == nil {
		t.Fatal("resolve() returned no disk")
	}
}

func TestResolveUnknownDiskFallsBackToLocal(t *testing.T) {
	config.Set("filesystems", config.M{
		"disks": config.M{"local": config.M{"driver": "local", "path": "./storage"}},
	})

	disk, err := resolve("nowhere")
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if disk == nil {
		t.Fatal("resolve() returned no disk")
	}
}

func TestResolveLocalWithoutAPath(t *testing.T) {
	config.Set("filesystems", config.M{
		"disks": config.M{"local": config.M{"driver": "local"}},
	})

	if _, err := resolve("local"); err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
}

// An unsupported driver is a mistake worth reporting, but as an error the
// caller can surface rather than a panic.
func TestResolveRejectsAnUnknownDriver(t *testing.T) {
	config.Set("filesystems", config.M{
		"disks": config.M{"weird": config.M{"driver": "ftp"}},
	})

	_, err := resolve("weird")
	if err == nil {
		t.Fatal("resolve() accepted an unsupported driver")
	}
}
