package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewFileSession(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSession(dir)
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestFileStoreCommitAndFind(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSession(dir)

	token := "test-token-123"
	data := []byte("session-data")
	expiry := time.Now().Add(1 * time.Hour)

	err := store.Commit(token, data, expiry)
	if err != nil {
		t.Fatal(err)
	}

	// Verify file exists
	path := filepath.Join(dir, token)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("session file was not created")
	}
}

func TestFileStoreFind(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSession(dir)

	token := "find-token"
	data := []byte("user-data")
	expiry := time.Now().Add(1 * time.Hour)

	store.Commit(token, data, expiry)

	got, found, err := store.Find(token)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("expected session to be found")
	}
	if string(got) != string(data) {
		t.Errorf("expected %s, got %s", data, got)
	}
}

func TestFileStoreFindExpired(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSession(dir)

	token := "expired-token"
	data := []byte("old-data")
	expiry := time.Now().Add(-1 * time.Hour)

	store.Commit(token, data, expiry)

	_, found, err := store.Find(token)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected expired session to not be found")
	}
}

func TestFileStoreFindMissing(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSession(dir)

	_, found, err := store.Find("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected missing session to not be found")
	}
}

func TestFileStoreDelete(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSession(dir)

	token := "delete-token"
	store.Commit(token, []byte("data"), time.Now().Add(1*time.Hour))

	err := store.Delete(token)
	if err != nil {
		t.Fatal(err)
	}

	_, found, _ := store.Find(token)
	if found {
		t.Error("expected deleted session to not be found")
	}
}

func TestFileStoreDeleteMissing(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSession(dir)

	_ = store.Delete("nonexistent")
}

func TestFileStoreDirCreated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "sessions")
	store := NewFileSession(dir)

	token := "new-dir-token"
	err := store.Commit(token, []byte("data"), time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("directory should have been created")
	}
}

func TestFileStoreOverwrite(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSession(dir)

	token := "overwrite-token"
	store.Commit(token, []byte("original"), time.Now().Add(1*time.Hour))
	store.Commit(token, []byte("updated"), time.Now().Add(1*time.Hour))

	got, _, _ := store.Find(token)
	if string(got) != "updated" {
		t.Errorf("expected 'updated', got %s", got)
	}
}

func TestFileStoreRejectsPathTraversalTokens(t *testing.T) {
	store := NewFileSession(t.TempDir())
	for _, token := range []string{"", ".", "..", "../outside", "nested/token", `nested\token`, "/tmp/session"} {
		t.Run(token, func(t *testing.T) {
			if err := store.Commit(token, []byte("data"), time.Now().Add(time.Hour)); err == nil {
				t.Fatal("expected invalid token error from Commit")
			}
			if _, _, err := store.Find(token); err == nil {
				t.Fatal("expected invalid token error from Find")
			}
			if err := store.Delete(token); err == nil {
				t.Fatal("expected invalid token error from Delete")
			}
		})
	}
}

func TestFileStoreAtomicReplacementAndPermissions(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSession(dir)
	token := "atomic-token"

	if err := store.Commit(token, []byte("first"), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(token, []byte("second"), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, found, err := store.Find(token)
	if err != nil || !found || string(got) != "second" {
		t.Fatalf("got %q, found %v, err %v", got, found, err)
	}
	info, err := os.Stat(filepath.Join(dir, token))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected session permissions 0600, got %o", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != token {
		t.Fatalf("expected only the committed session file, got %v", entries)
	}
}

func TestFileStoreExpiredSessionIsCleanedUp(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSession(dir)
	token := "expired-cleanup"
	if err := store.Commit(token, []byte("old"), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	if _, found, err := store.Find(token); err != nil || found {
		t.Fatalf("expected expired session, found %v, err %v", found, err)
	}
	if _, err := os.Stat(filepath.Join(dir, token)); !os.IsNotExist(err) {
		t.Fatalf("expected expired session file to be removed, got %v", err)
	}
}
