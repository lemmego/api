package db

import (
	"github.com/lemmego/api/config"
	"github.com/lemmego/gpa"
)

func SqlProvider(instance ...string) gpa.SQLProvider {
	return config.Get("sql.gpaprovider").(func(...string) gpa.SQLProvider)(instance...)
}

func SetSqlProvider(provider gpa.SQLProvider) {
	config.Set("sql.gpaprovider", func(...string) gpa.SQLProvider {
		return provider
	})
}

func DocumentProvider(instance ...string) gpa.DocumentProvider {
	return config.Get("document.gpaprovider").(func(...string) gpa.DocumentProvider)(instance...)
}

func SetDocumentProvider(provider gpa.DocumentProvider) {
	config.Set("document.gpaprovider", func(...string) gpa.DocumentProvider {
		return provider
	})
}

func KeyValueProvider(instance ...string) gpa.Provider {
	return config.Get("keyvalue.gpaprovider").(func(...string) gpa.KeyValueProvider)(instance...)
}

func SetKeyValueProvider(provider gpa.KeyValueProvider) {
	config.Set("keyvalue.gpaprovider", func(...string) gpa.KeyValueProvider {
		return provider
	})
}
