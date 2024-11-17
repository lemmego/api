package middleware

import (
	"dario.cat/mergo"
	"github.com/go-chi/httplog/v2"
	"github.com/lemmego/api/app"
	"log/slog"
	"os"
)

func Logger(options ...httplog.Options) app.HTTPMiddleware {
	name := os.Getenv("APP_NAME")
	if name == "" {
		name = "lemmego"
	}

	defaultOpts := httplog.Options{
		LogLevel:         slog.LevelDebug,
		Concise:          true,
		RequestHeaders:   true,
		MessageFieldName: "message",
		TimeFieldFormat:  "[15:04:05.000]",
	}

	if len(options) > 0 {
		err := mergo.Merge(&defaultOpts, options[0], mergo.WithOverride, mergo.WithoutDereference)
		if err != nil {
			slog.Info(err.Error())
		}
	}

	logger := httplog.NewLogger(name, options[0])

	return httplog.RequestLogger(logger)
}
