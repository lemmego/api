package encryption

import (
	"bytes"
	"testing"
)

func TestNewEncrypterInvalidKey(t *testing.T) {
	_, err := NewEncrypter([]byte("short"))
	if err == nil {
		t.Error("expected error for short key")
	}
}

func TestNewEncrypterValidKey(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	e, err := NewEncrypter(key)
	if err != nil {
		t.Fatal(err)
	}
	if e == nil {
		t.Fatal("expected non-nil encrypter")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	e, _ := NewEncrypter(key)

	plaintext := []byte("hello world")
	ciphertext, err := e.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(plaintext, ciphertext) {
		t.Error("ciphertext should not equal plaintext")
	}

	decrypted, err := e.Decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("expected %s, got %s", plaintext, decrypted)
	}
}

func TestEncryptStringDecryptString(t *testing.T) {
	key := make([]byte, 32)
	e, _ := NewEncrypter(key)

	original := "secret message"
	encoded, err := e.EncryptString(original)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := e.DecryptString(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded != original {
		t.Errorf("expected %s, got %s", original, decoded)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 0xFF

	e1, _ := NewEncrypter(key1)
	e2, _ := NewEncrypter(key2)

	ciphertext, err := e1.Encrypt([]byte("test"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = e2.Decrypt(ciphertext)
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestDecryptTampered(t *testing.T) {
	key := make([]byte, 32)
	e, _ := NewEncrypter(key)

	ciphertext, err := e.Encrypt([]byte("test"))
	if err != nil {
		t.Fatal(err)
	}

	ciphertext[len(ciphertext)-1] ^= 0xFF

	_, err = e.Decrypt(ciphertext)
	if err == nil {
		t.Error("expected error when decrypting tampered ciphertext")
	}
}

func TestEncryptEmpty(t *testing.T) {
	key := make([]byte, 32)
	e, _ := NewEncrypter(key)

	ciphertext, err := e.Encrypt([]byte{})
	if err != nil {
		t.Fatal(err)
	}

	decrypted, err := e.Decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}

	if len(decrypted) != 0 {
		t.Error("expected empty decrypted result")
	}
}

func TestMultipleEncryptionsDifferent(t *testing.T) {
	key := make([]byte, 32)
	e, _ := NewEncrypter(key)

	plaintext := []byte("same data")
	c1, _ := e.Encrypt(plaintext)
	c2, _ := e.Encrypt(plaintext)

	if bytes.Equal(c1, c2) {
		t.Error("two encryptions of same data should differ (nonce)")
	}
}
