package db

import (
	"github.com/lemmego/api/config"
	"github.com/lemmego/gpa"
)

func DefaultSQLProvider() gpa.Provider {
	return config.Get("sql").(config.M)["default_provider"].(gpa.Provider)
}

func DefaultDocumentProvider() gpa.Provider {
	return config.Get("document").(config.M)["default_provider"].(gpa.Provider)
}

func DefaultKeyValueProvider() gpa.Provider {
	return config.Get("keyvalue").(config.M)["default_provider"].(gpa.Provider)
}
