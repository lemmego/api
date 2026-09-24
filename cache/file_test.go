package cache

import (
	"testing"
	"time"
)

func TestFileStoreContract(t *testing.T) {
	store := NewFileStore(t.TempDir())

	store.Put("name", "Lemmego", 60)
	if got := store.Get("name"); got != "Lemmego" {
		t.Fatalf("Get() = %v, want Lemmego", got)
	}

	store.PutMany(map[string]interface{}{"one": 1, "two": 2}, 60)
	many := store.Many([]string{"one", "two", "missing"})
	if len(many) != 2 || many["one"] != 1 || many["two"] != 2 {
		t.Fatalf("Many() = %#v, want the two stored values", many)
	}

	if !store.Forget("name") || store.Get("name") != nil {
		t.Fatal("Forget() did not remove the value")
	}
	if !store.Flush() || store.Get("one") != nil || store.Get("two") != nil {
		t.Fatal("Flush() did not remove all values")
	}
}

func TestFileStoreExpiry(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	store.Put("short-lived", "value", 1)

	if got := NewFileStore(dir).Get("short-lived"); got != "value" {
		t.Fatalf("value was not available to another store: %v", got)
	}

	time.Sleep(1100 * time.Millisecond)
	if got := store.Get("short-lived"); got != nil {
		t.Fatalf("expired Get() = %v, want nil", got)
	}
}

func TestFileStoreForeverAndCounters(t *testing.T) {
	store := NewFileStore(t.TempDir())
	store.Forever("forever", "value")
	if got := store.Get("forever"); got != "value" {
		t.Fatalf("Forever() value = %v, want value", got)
	}

	if got := store.Increment("count", 2); got != 2 {
		t.Fatalf("Increment() = %d, want 2", got)
	}
	if got := store.Decrement("count", 1); got != 1 {
		t.Fatalf("Decrement() = %d, want 1", got)
	}
}
