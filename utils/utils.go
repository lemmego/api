package utils

import (
	"encoding/json"
	"math/rand"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"golang.org/x/crypto/bcrypt"
)

// GenerateRandomString generates a random string of a given length using the characters provided.
func GenerateRandomString(length int) string {
	characters := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.Seed(time.Now().UnixNano())

	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = characters[rand.Intn(len(characters))]
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
func Bcrypt(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
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
