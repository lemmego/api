package utils

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"math/big"

	_ "github.com/joho/godotenv/autoload"
	"github.com/lemmego/api/config"
	"golang.org/x/crypto/bcrypt"
)

// GenerateRandomString generates a random string of a given length using the characters provided.
func GenerateRandomString(length int) string {
	characters := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if length <= 0 {
		return ""
	}
	max := big.NewInt(int64(len(characters)))

	result := make([]byte, length)
	for i := 0; i < length; i++ {
		index, err := rand.Int(rand.Reader, max)
		if err != nil {
			return ""
		}
		result[i] = characters[index.Int64()]
	}

	return string(result)
}

// PrettyPrint converts a map to a pretty-printed JSON string
func PrettyPrint(data map[string]interface{}) (string, error) {
	// Marshal the map to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}

	// Convert the JSON byte slice to a string
	return string(jsonData), nil
}

// Bcrypt hashes a string
func Bcrypt(password string, rounds ...int) (string, error) {
	bcryptRounds := 10
	if len(rounds) > 0 {
		bcryptRounds = rounds[0]
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptRounds)
	return string(bytes), err
}

// StructToMap converts any struct to map[string]interface{}
func StructToMap(obj interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(obj) // Convert to JSON
	if err != nil {
		return nil, err
	}
	var ret map[string]interface{}
	err = json.Unmarshal(data, &ret) // Convert back to map
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// The paths below say where generated code goes. They are read from the
// application's config, which a project may not have loaded — the generators
// run outside a project too — so each falls back to the conventional location
// rather than asserting. They previously asserted the value straight out of
// the config and panicked when the app section was absent or a key was
// missing.
func projectPath(key, fallback string) string {
	if value, ok := config.Get("app." + key).(string); ok && value != "" {
		return value
	}
	return fallback
}

func ConfigPath() string     { return projectPath("config_path", "./internal/configs") }
func CommandPath() string    { return projectPath("command_path", "./internal/commands") }
func HandlerPath() string    { return projectPath("handler_path", "./internal/handlers") }
func InputPath() string      { return projectPath("input_path", "./internal/inputs") }
func MiddlewarePath() string { return projectPath("middleware_path", "./internal/middleware") }
func MigrationPath() string  { return projectPath("migration_path", "./internal/migrations") }
func ModelPath() string      { return projectPath("model_path", "./internal/models") }
func RoutePath() string      { return projectPath("route_path", "./internal/routes") }

func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	return key, err
}

func EncodeToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
