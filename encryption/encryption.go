package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"sync"
)

var (
	encrypter *Encrypter
	once      sync.Once
)

func initEncrypter() {
	appKey := os.Getenv("APP_KEY")
	if appKey == "" {
		panic("APP_KEY environment variable not set")
	}
	key, err := base64.StdEncoding.DecodeString(appKey)
	if err != nil {
		panic(err)
	}
	val, err := NewEncrypter(key)
	if err != nil {
		panic(err)
	}
	encrypter = val
}

func Get() *Encrypter {
	once.Do(initEncrypter)
	return encrypter
}

// Encrypt takes plaintext and returns a base64 encoded string of the ciphertext
func Encrypt(plaintext []byte) (string, error) {
	ciphertext, err := Get().Encrypt(plaintext)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt takes a base64 encoded ciphertext string and returns the plaintext bytes
func Decrypt(encodedCiphertext string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return nil, err
	}
	return Get().Decrypt(ciphertext)
}

// Encrypter represents an AEAD encrypter/decrypter
type Encrypter struct {
	key []byte
}

// NewEncrypter creates a new instance of Encrypter with the provided key
func NewEncrypter(key []byte) (*Encrypter, error) {
	if len(key) != 32 { // AES-256 requires a 32 byte key
		return nil, errors.New("key must be 32 bytes long for AES-256")
	}
	return &Encrypter{key: key}, nil
}

// Encrypt encrypts the given plaintext
func (e *Encrypter) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return append(nonce, ciphertext...), nil
}

// Decrypt decrypts the provided ciphertext
func (e *Encrypter) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("malformed ciphertext")
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// EncryptString encrypts a string
func (e *Encrypter) EncryptString(plaintext string) (string, error) {
	ciphertext, err := e.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptString decrypts a base64 encoded string
func (e *Encrypter) DecryptString(encodedCiphertext string) (string, error) {
	plaintext, err := e.DecryptStringHelper(encodedCiphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// Helper function for DecryptString to keep method signatures consistent with package level functions
func (e *Encrypter) DecryptStringHelper(encodedCiphertext string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return nil, err
	}

	plaintext, err := e.Decrypt(ciphertext)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
