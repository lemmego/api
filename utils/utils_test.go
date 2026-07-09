package utils

import (
	"strings"
	"testing"

	"github.com/lemmego/api/config"
)

func TestGenerateRandomString(t *testing.T) {
	s := GenerateRandomString(16)
	if len(s) != 16 {
		t.Errorf("expected length 16, got %d", len(s))
	}
}

func TestGenerateRandomStringUnique(t *testing.T) {
	a := GenerateRandomString(32)
	b := GenerateRandomString(32)
	if a == b {
		t.Error("two random strings should differ")
	}
}

func TestGenerateRandomStringEmpty(t *testing.T) {
	s := GenerateRandomString(0)
	if s != "" {
		t.Errorf("expected empty string, got %s", s)
	}
}

func TestBcryptHashAndVerify(t *testing.T) {
	password := "my-secret-password"
	hash, err := Bcrypt(password)
	if err != nil {
		t.Fatal(err)
	}
	if hash == password {
		t.Error("hash should differ from password")
	}
	if !strings.HasPrefix(hash, "$2a$") {
		t.Errorf("expected bcrypt prefix, got %s", hash[:4])
	}
}

func TestBcryptCustomRounds(t *testing.T) {
	hash, err := Bcrypt("password", 10)
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}
}

func TestStructToMap(t *testing.T) {
	type User struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Age   int    `json:"age"`
	}
	u := User{Name: "John", Email: "john@test.com", Age: 30}
	m, err := StructToMap(u)
	if err != nil {
		t.Fatal(err)
	}
	if m["name"] != "John" {
		t.Errorf("expected John, got %v", m["name"])
	}
	if m["age"] != float64(30) {
		t.Errorf("expected 30, got %v", m["age"])
	}
}

func TestStructToMapNil(t *testing.T) {
	m, err := StructToMap(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(key))
	}
}

func TestGenerateKeyUnique(t *testing.T) {
	a, _ := GenerateKey()
	b, _ := GenerateKey()
	if string(a) == string(b) {
		t.Error("two keys should differ")
	}
}

func TestEncodeToBase64(t *testing.T) {
	data := []byte("hello world")
	encoded := EncodeToBase64(data)
	if encoded == "" {
		t.Error("expected non-empty base64")
	}
	if !strings.Contains(encoded, "aGVsbG8gd29ybGQ") {
		t.Errorf("expected base64 of 'hello world', got %s", encoded)
	}
}

func TestPrettyPrint(t *testing.T) {
	data := map[string]interface{}{
		"name": "test",
		"num":  42,
	}
	s, err := PrettyPrint(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "name") || !strings.Contains(s, "test") {
		t.Errorf("expected JSON with name/test, got %s", s)
	}
}

func TestConfigPath(t *testing.T) {
	config.Set("app", config.M{
		"config_path":     "./internal/configs",
		"command_path":    "./internal/commands",
		"handler_path":    "./internal/handlers",
		"input_path":      "./internal/inputs",
		"middleware_path": "./internal/middlewares",
		"migration_path":  "./internal/migrations",
		"model_path":      "./internal/models",
		"route_path":      "./internal/routes",
	})
	if p := ConfigPath(); p != "./internal/configs" {
		t.Errorf("expected ./internal/configs, got %s", p)
	}
	if p := CommandPath(); p != "./internal/commands" {
		t.Errorf("expected ./internal/commands, got %s", p)
	}
	if p := HandlerPath(); p != "./internal/handlers" {
		t.Errorf("expected ./internal/handlers, got %s", p)
	}
	if p := RoutePath(); p != "./internal/routes" {
		t.Errorf("expected ./internal/routes, got %s", p)
	}
}
