package app

import (
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
