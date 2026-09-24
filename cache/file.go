// The file cache driver implementation for the cache package.
package cache

import (
	"crypto/sha256"
	"encoding/gob"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileStore struct {
	prefix string
	mu     sync.RWMutex
}

type fileCacheEntry struct {
	ExpiresAt time.Time
	Value     interface{}
}

func NewFileStore(prefix string) *FileStore {
	if prefix == "" {
		prefix = "storage/cache"
	}
	if err := os.MkdirAll(prefix, 0700); err != nil {
		panic(err)
	}
	return &FileStore{prefix: prefix}
}

func (f *FileStore) Get(key string) interface{} {
	f.mu.RLock()
	entry, ok := f.read(key)
	f.mu.RUnlock()
	if !ok {
		return nil
	}
	if !entry.ExpiresAt.IsZero() && !time.Now().Before(entry.ExpiresAt) {
		f.mu.Lock()
		_ = os.Remove(f.filename(key))
		f.mu.Unlock()
		return nil
	}
	return entry.Value
}

func (f *FileStore) Many(keys []string) map[string]interface{} {
	values := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		if value := f.Get(key); value != nil {
			values[key] = value
		}
	}
	return values
}

func (f *FileStore) Put(key string, value interface{}, seconds int) {
	if seconds <= 0 {
		_ = f.Forget(key)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.write(key, fileCacheEntry{ExpiresAt: time.Now().Add(time.Duration(seconds) * time.Second), Value: value})
}

func (f *FileStore) PutMany(values map[string]interface{}, seconds int) {
	if seconds <= 0 {
		for key := range values {
			_ = f.Forget(key)
		}
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	expiresAt := time.Now().Add(time.Duration(seconds) * time.Second)
	for key, value := range values {
		f.write(key, fileCacheEntry{ExpiresAt: expiresAt, Value: value})
	}
}

func (f *FileStore) Increment(key string, value int) int {
	return f.change(key, value)
}

func (f *FileStore) Decrement(key string, value int) int {
	return f.change(key, -value)
}

func (f *FileStore) Forever(key string, value interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.write(key, fileCacheEntry{Value: value})
}

func (f *FileStore) Forget(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	err := os.Remove(f.filename(key))
	return err == nil || os.IsNotExist(err)
}

func (f *FileStore) Flush() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	entries, err := os.ReadDir(f.prefix)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(f.prefix, entry.Name())); err != nil {
			return false
		}
	}
	return true
}

func (f *FileStore) GetPrefix() string {
	return f.prefix
}

func (f *FileStore) change(key string, delta int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	current := 0
	if entry, ok := f.read(key); ok && (entry.ExpiresAt.IsZero() || time.Now().Before(entry.ExpiresAt)) {
		var numeric bool
		current, numeric = entry.Value.(int)
		if !numeric {
			panic("cache: increment/decrement requires an int value")
		}
	}
	current += delta
	f.write(key, fileCacheEntry{Value: current})
	return current
}

func (f *FileStore) filename(key string) string {
	return filepath.Join(f.prefix, "cache-"+stringHash(key))
}

func (f *FileStore) read(key string) (fileCacheEntry, bool) {
	file, err := os.Open(f.filename(key))
	if err != nil {
		return fileCacheEntry{}, false
	}
	defer file.Close()
	var entry fileCacheEntry
	if err := gob.NewDecoder(file).Decode(&entry); err != nil {
		return fileCacheEntry{}, false
	}
	return entry, true
}

func (f *FileStore) write(key string, entry fileCacheEntry) {
	tmp, err := os.CreateTemp(f.prefix, ".cache-")
	if err != nil {
		panic(err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		panic(err)
	}
	if err := gob.NewEncoder(tmp).Encode(entry); err != nil {
		_ = tmp.Close()
		panic(err)
	}
	if err := tmp.Close(); err != nil {
		panic(err)
	}
	if err := os.Rename(tmpName, f.filename(key)); err != nil {
		panic(err)
	}
}

func stringHash(value string) string {
	return stringHashBytes([]byte(value))
}

func stringHashBytes(value []byte) string {
	hash := sha256.Sum256(value)
	const hex = "0123456789abcdef"
	result := make([]byte, len(hash)*2)
	for i, value := range hash {
		result[i*2] = hex[value>>4]
		result[i*2+1] = hex[value&0xf]
	}
	return string(result)
}
