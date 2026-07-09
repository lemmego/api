package req

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWantsJSON(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "application/json")
	if !WantsJSON(r) {
		t.Error("expected WantsJSON true for application/json accept")
	}

	r2 := httptest.NewRequest("GET", "/", nil)
	r2.Header.Set("Accept", "text/html")
	if WantsJSON(r2) {
		t.Error("expected WantsJSON false for text/html accept")
	}
}

func TestWantsHTML(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "text/html")
	if !WantsHTML(r) {
		t.Error("expected WantsHTML true for text/html accept")
	}
}

func TestWantsXML(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "application/xml")
	if !WantsXML(r) {
		t.Error("expected WantsXML true for application/xml accept")
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

func (m *mockContext) Request() *http.Request      { return m.r }
func (m *mockContext) ResponseWriter() http.ResponseWriter { return m.w }
func (m *mockContext) Get(key string) any          { return nil }
func (m *mockContext) Set(key string, value any)   {}

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
