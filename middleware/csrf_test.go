package middleware

import (
	"sync"
	"testing"
)

func TestShouldExcludePathConcurrentCacheAccess(t *testing.T) {
	patterns := []string{`^/api/`, `^/hooks/.*`, `[invalid`}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				shouldExcludePath("/api/users", patterns)
			}
		}()
	}
	wg.Wait()
}
