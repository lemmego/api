package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/httplog/v2"
)

func TestLoggerUsesDefaultsWithoutOptions(t *testing.T) {
	middleware := Logger()
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	writer := httptest.NewRecorder()
	handler.ServeHTTP(writer, request)

	if writer.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, writer.Code)
	}
}

func TestLoggerMergesOptionsWithDefaults(t *testing.T) {
	if Logger(httplog.Options{MessageFieldName: "event"}) == nil {
		t.Fatal("expected logger middleware")
	}
}
