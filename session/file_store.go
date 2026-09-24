package session

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultDir = "storage/session"

type FileStore struct {
	dir string
}

func (fs *FileStore) Delete(token string) error {
	filename, err := fs.filename(token)
	if err != nil {
		return err
	}
	return os.Remove(filename)
}

func (fs *FileStore) Find(token string) ([]byte, bool, error) {
	filename, err := fs.filename(token)
	if err != nil {
		return nil, false, err
	}
	f, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, false, err
	}

	parts := strings.SplitN(string(data), "|", 2)
	if len(parts) != 2 {
		return nil, false, fmt.Errorf("invalid file format")
	}

	expiry, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		return nil, false, err
	}

	if time.Now().After(expiry) {
		os.Remove(filename) // Clean up expired session
		return nil, false, nil
	}

	sessionData, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false, err
	}

	return sessionData, true, nil
}

func (fs *FileStore) Commit(token string, b []byte, expiry time.Time) error {
	filename, err := fs.filename(token)
	if err != nil {
		return err
	}

	data := fmt.Sprintf("%s|%s", expiry.Format(time.RFC3339), base64.StdEncoding.EncodeToString(b))
	tmp, err := os.CreateTemp(fs.dir, ".session-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.WriteString(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filename)
}

func (fs *FileStore) filename(token string) (string, error) {
	if token == "" || token == "." || token == ".." ||
		strings.ContainsAny(token, `/\\`) || filepath.IsAbs(token) || filepath.Clean(token) != token {
		return "", fmt.Errorf("invalid session token")
	}
	return filepath.Join(fs.dir, token), nil
}

func NewFileSession(directoryPath string) *FileStore {
	if directoryPath == "" {
		directoryPath = defaultDir
	}
	err := os.MkdirAll(directoryPath, 0755)
	if err != nil {
		panic(err)
	}
	return &FileStore{dir: directoryPath}
}
