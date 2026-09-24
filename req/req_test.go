package req

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWantsMediaTypes(t *testing.T) {
	tests := []struct {
		name       string
		accept     string
		acceptMore string
		json       bool
		html       bool
		xml        bool
	}{
		{name: "API JSON", accept: "application/json", json: true},
		{name: "browser", accept: "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", json: true, html: true, xml: true},
		{name: "quality values", accept: "application/json;q=0.2, text/html;q=0.9", json: true, html: true},
		{name: "zero quality", accept: "application/json;q=0, text/html", html: true},
		{name: "specific zero overrides wildcard", accept: "application/json;q=0, */*;q=0.8", html: true, xml: true},
		{name: "application wildcard", accept: "application/*", json: true, xml: true},
		{name: "text wildcard", accept: "text/*", html: true, xml: true},
		{name: "all wildcard", accept: "*/*", json: true, html: true, xml: true},
		{name: "parameters", accept: "application/json; charset=utf-8; q=0.7", json: true},
		{name: "vendor suffix", accept: "application/vnd.api+json", json: true},
		{name: "multiple accept headers", accept: "text/plain", acceptMore: "application/xml", xml: true},
		{name: "malformed among valid", accept: "not-a-media-type, application/json; q=0.5", json: true},
		{name: "malformed quality", accept: "application/json;q=wat"},
		{name: "invalid quality range", accept: "text/html;q=1.1, application/xml;q=-0.1"},
		{name: "empty", accept: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Accept", tt.accept)
			if tt.acceptMore != "" {
				r.Header.Add("Accept", tt.acceptMore)
			}

			if got := WantsJSON(r); got != tt.json {
				t.Errorf("WantsJSON() = %v, want %v", got, tt.json)
			}
			if got := WantsHTML(r); got != tt.html {
				t.Errorf("WantsHTML() = %v, want %v", got, tt.html)
			}
			if got := WantsXML(r); got != tt.xml {
				t.Errorf("WantsXML() = %v, want %v", got, tt.xml)
			}
		})
	}
}

func TestHasJSON(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("Content-Type", "application/json")
	if !HasJSON(r) {
		t.Error("expected HasJSON true for application/json")
	}

	r2 := httptest.NewRequest("POST", "/", nil)
	r2.Header.Set("Content-Type", "text/plain")
	if HasJSON(r2) {
		t.Error("expected HasJSON false for text/plain")
	}
}

func TestHasFormData(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("Content-Type", "multipart/form-data")
	if !HasMultiPart(r) {
		t.Error("expected HasMultiPart true for multipart/form-data")
	}
}

func TestHasFormURLEncoded(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if !HasFormUrlEncoded(r) {
		t.Error("expected HasFormUrlEncoded true")
	}
}

func TestDecodeJSONBodyValid(t *testing.T) {
	type Input struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	body := bytes.NewBufferString(`{"name":"John","email":"john@test.com"}`)
	r := httptest.NewRequest("POST", "/", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var input Input
	err := DecodeJSONBody(w, r, &input)
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != "John" {
		t.Errorf("expected John, got %s", input.Name)
	}
}

func TestDecodeJSONBodyInvalid(t *testing.T) {
	r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{invalid json}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var input map[string]any
	err := DecodeJSONBody(w, r, &input)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	var malformed *MalformedRequest
	if !strings.Contains(err.Error(), "malformed") && !strings.Contains(err.Error(), "badly") {
		t.Logf("got error: %v", err)
	}
	_ = malformed
}

func TestDecodeJSONBodyUnknownField(t *testing.T) {
	type Input struct {
		Name string `json:"name"`
	}
	body := bytes.NewBufferString(`{"name":"John","extra":"field"}`)
	r := httptest.NewRequest("POST", "/", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var input Input
	err := DecodeJSONBody(w, r, &input)
	if err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestDecodeJSONBodyEmpty(t *testing.T) {
	r := httptest.NewRequest("POST", "/", bytes.NewBufferString(``))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var input map[string]any
	err := DecodeJSONBody(w, r, &input)
	if err == nil {
		t.Error("expected error for empty body")
	}
}

func TestParseInputJSON(t *testing.T) {
	type Input struct {
		Title string `json:"title"`
	}
	body := bytes.NewBufferString(`{"title":"hello"}`)
	r := httptest.NewRequest("POST", "/", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mockCtx := &mockContext{r: r, w: w}
	var input Input
	err := ParseInput(mockCtx, &input)
	if err != nil {
		t.Fatal(err)
	}
	if input.Title != "hello" {
		t.Errorf("expected hello, got %s", input.Title)
	}
}

type mockContext struct {
	r *http.Request
	w http.ResponseWriter
}

func (m *mockContext) Request() *http.Request              { return m.r }
func (m *mockContext) ResponseWriter() http.ResponseWriter { return m.w }
func (m *mockContext) Get(key string) any                  { return nil }
func (m *mockContext) Set(key string, value any)           {}

func TestDecodeJSONBodyStrict(t *testing.T) {
	jsonStr := `{"valid": true}`
	r := httptest.NewRequest("POST", "/", bytes.NewBufferString(jsonStr))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var result map[string]any
	err := DecodeJSONBody(w, r, &result)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(result)
	if !strings.Contains(string(b), "true") {
		t.Errorf("expected valid=true in result, got %s", b)
	}
}
