package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestXMLBytes(t *testing.T) {
	w := httptest.NewRecorder()
	c := &ctx{writer: w}

	err := c.XML([]byte(`<status>ok</status>`))
	if err != nil {
		t.Fatal(err)
	}

	body := w.Body.String()
	ct := w.Header().Get("Content-Type")

	if ct != "application/xml" {
		t.Errorf("expected application/xml, got %s", ct)
	}
	if !strings.Contains(body, "<status>ok</status>") {
		t.Errorf("expected <status>ok</status>, got %s", body)
	}
}

func TestXMLString(t *testing.T) {
	w := httptest.NewRecorder()
	c := &ctx{writer: w}

	err := c.XML("<root><item>hello</item></root>")
	if err != nil {
		t.Fatal(err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<item>hello</item>") {
		t.Errorf("expected <item>hello</item>, got %s", body)
	}
}

func TestXMLMap(t *testing.T) {
	w := httptest.NewRecorder()
	c := &ctx{writer: w}

	err := c.XML(M{"status": "ok", "count": 42})
	if err != nil {
		t.Fatal(err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<response>") {
		t.Errorf("expected <response> root element, got %s", body)
	}
	if !strings.Contains(body, "<status>ok</status>") {
		t.Errorf("expected <status>ok</status>, got %s", body)
	}
	if !strings.Contains(body, "<count>42</count>") {
		t.Errorf("expected <count>42</count>, got %s", body)
	}
}

func TestXMLCustomStruct(t *testing.T) {
	type Result struct {
		Message string `xml:"message"`
		Code    int    `xml:"code"`
	}

	w := httptest.NewRecorder()
	c := &ctx{writer: w}

	err := c.XML(Result{Message: "hello", Code: 200})
	if err != nil {
		t.Fatal(err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<message>hello</message>") {
		t.Errorf("expected <message>hello</message>, got %s", body)
	}
	if !strings.Contains(body, "<code>200</code>") {
		t.Errorf("expected <code>200</code>, got %s", body)
	}
}

func TestXMLStatus(t *testing.T) {
	w := httptest.NewRecorder()
	c := &ctx{writer: w, status: 201}

	err := c.XML(M{"created": true})
	if err != nil {
		t.Fatal(err)
	}

	if w.Code != 201 {
		t.Errorf("expected status 201, got %d", w.Code)
	}
}

func TestXMLEmptyMap(t *testing.T) {
	w := httptest.NewRecorder()
	c := &ctx{writer: w}

	err := c.XML(M{})
	if err != nil {
		t.Fatal(err)
	}

	body := w.Body.String()
	if body != "<response></response>" {
		t.Errorf("expected <response></response>, got %s", body)
	}
}

func TestXMLSpecialChars(t *testing.T) {
	w := httptest.NewRecorder()
	c := &ctx{writer: w}

	err := c.XML(M{"msg": "a < b & c > d"})
	if err != nil {
		t.Fatal(err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "&lt;") || !strings.Contains(body, "&amp;") || !strings.Contains(body, "&gt;") {
		t.Errorf("expected escaped XML entities, got: %s", body)
	}
	if strings.Contains(body, "<msg>a < b") {
		t.Errorf("expected < to be escaped, got: %s", body)
	}
}

func TestJSON(t *testing.T) {
	w := httptest.NewRecorder()
	c := &ctx{writer: w}

	err := c.JSON(M{"key": "value", "num": 42})
	if err != nil {
		t.Fatal(err)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"key"`) || !strings.Contains(body, `"value"`) {
		t.Errorf("expected JSON body, got %s", body)
	}
}

func TestJSONPreservesStatus(t *testing.T) {
	w := httptest.NewRecorder()
	c := &ctx{writer: w, status: 201}

	err := c.JSON(M{"created": true})
	if err != nil {
		t.Fatal(err)
	}

	if w.Code != 201 {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestText(t *testing.T) {
	w := httptest.NewRecorder()
	c := &ctx{writer: w}

	err := c.Text([]byte("hello world"))
	if err != nil {
		t.Fatal(err)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/plain" {
		t.Errorf("expected text/plain, got %s", ct)
	}
	if w.Body.String() != "hello world" {
		t.Errorf("expected 'hello world', got %s", w.Body.String())
	}
}

func TestHTML(t *testing.T) {
	w := httptest.NewRecorder()
	c := &ctx{writer: w}

	err := c.HTML([]byte("<h1>Title</h1>"))
	if err != nil {
		t.Fatal(err)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/html" {
		t.Errorf("expected text/html, got %s", ct)
	}
	if w.Body.String() != "<h1>Title</h1>" {
		t.Errorf("expected '<h1>Title</h1>', got %s", w.Body.String())
	}
}

func TestRedirect(t *testing.T) {
	w := httptest.NewRecorder()
	c := &ctx{writer: w}

	err := c.Redirect("/home")
	if err != nil {
		t.Fatal(err)
	}

	loc := w.Header().Get("Location")
	if loc != "/home" {
		t.Errorf("expected Location /home, got %s", loc)
	}
	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}
}

func TestBack(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Referer", "/previous")
	c := &ctx{writer: w, request: r}

	err := c.Back()
	if err != nil {
		t.Fatal(err)
	}

	loc := w.Header().Get("Location")
	if loc != "/previous" {
		t.Errorf("expected /previous, got %s", loc)
	}
}

func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	c := &ctx{writer: w, request: r}

	err := c.Error(http.StatusNotFound, errors.New("not found"))
	if err != nil {
		t.Fatal(err)
	}

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not found") {
		t.Errorf("expected error message in body, got %s", w.Body.String())
	}
}

func TestErrorWantsJSON(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "application/json")
	c := &ctx{writer: w, request: r}

	err := c.Error(http.StatusBadRequest, errors.New("bad request"))
	if err != nil {
		t.Fatal(err)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}

func TestGetSet(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	c := &ctx{request: r}

	c.Set("user", "tanmay")
	val := c.Get("user")
	if val != "tanmay" {
		t.Errorf("expected 'tanmay', got %v", val)
	}
}

func TestGetSetNil(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	c := &ctx{request: r}

	val := c.Get("nonexistent")
	if val != nil {
		t.Errorf("expected nil, got %v", val)
	}
}

func TestStatus(t *testing.T) {
	c := &ctx{}

	if c.Status() != 0 {
		t.Errorf("expected 0, got %d", c.Status())
	}

	c.SetStatus(200)
	if c.Status() != 200 {
		t.Errorf("expected 200, got %d", c.Status())
	}
}

func TestWriteStatus(t *testing.T) {
	w := httptest.NewRecorder()
	c := &ctx{writer: w}

	c.WriteStatus(http.StatusTeapot)
	if w.Code != http.StatusTeapot {
		t.Errorf("expected 418, got %d", w.Code)
	}
}

func TestCookieRoundTrip(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	c := &ctx{writer: w, request: r}

	got := c.Cookie("test")
	if got != nil {
		t.Errorf("expected nil for missing cookie, got %v", got)
	}

	c.SetCookie(&http.Cookie{Name: "session", Value: "abc123"})

	r.Header.Set("Cookie", "session=abc123")
	got = c.Cookie("session")
	if got == nil || got.Value != "abc123" {
		t.Errorf("expected session cookie with abc123, got %v", got)
	}
}

func TestIsReading(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{"GET", true},
		{"HEAD", true},
		{"OPTIONS", true},
		{"POST", false},
		{"PUT", false},
		{"DELETE", false},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(tt.method, "/", nil)
		c := &ctx{request: r}
		if got := c.IsReading(); got != tt.want {
			t.Errorf("IsReading(%s) = %v, want %v", tt.method, got, tt.want)
		}
	}
}

func TestParam(t *testing.T) {
	r := httptest.NewRequest("GET", "/users/42", nil)
	r.SetPathValue("id", "42")
	c := &ctx{request: r}

	if got := c.Param("id"); got != "42" {
		t.Errorf("expected 42, got %s", got)
	}
}

func TestQuery(t *testing.T) {
	r := httptest.NewRequest("GET", "/search?q=golang", nil)
	c := &ctx{request: r}

	if got := c.Query("q"); got != "golang" {
		t.Errorf("expected golang, got %s", got)
	}
	if got := c.Query("missing"); got != "" {
		t.Errorf("expected empty, got %s", got)
	}
}
