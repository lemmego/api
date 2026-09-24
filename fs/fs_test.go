package fs

import (
	"sync"
	"testing"

	"github.com/lemmego/api/config"
)

func TestDiskConcurrentCacheAccess(t *testing.T) {
	config.GetInstance().SetConfigMap(config.M{
		"filesystems": config.M{
			"disks": config.M{
				"local": config.M{"driver": "local", "path": t.TempDir()},
			},
		},
	})

	fm := NewFileSystem()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				disk, err := fm.Disk("local")
				if err != nil || disk == nil {
					t.Errorf("Disk() = %v, %v", disk, err)
				}
			}
		}()
	}
	wg.Wait()
}
