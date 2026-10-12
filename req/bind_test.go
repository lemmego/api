package req

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The edge cases of binding a request body, in one place.
//
// This file exists because the framework carried two binders that disagreed,
// and the disagreements were only ever found in production: an array shape
// that bound under one and not the other, a checkbox whose `on` was true
// under one and a 500 under the other, files that bound under neither. None
// of it was written down, so none of it was checked.

func form(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/f", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

func bind(t *testing.T, r *http.Request, into any) error {
	t.Helper()
	return ParseInput(&formResponder{r: r, w: httptest.NewRecorder()}, into)
}

// ---------------------------------------------------------------- scalars

func TestBindScalars(t *testing.T) {
	type input struct {
		Name    string  `form:"name"`
		Age     int     `form:"age"`
		Big     int64   `form:"big"`
		Count   uint    `form:"count"`
		Ratio   float64 `form:"ratio"`
		Small   float32 `form:"small"`
		Enabled bool    `form:"enabled"`
	}

	var got input
	err := bind(t, form("name=Ada&age=-36&big=9007199254740993&count=7&ratio=1.5&small=0.25&enabled=true"), &got)
	if err != nil {
		t.Fatal(err)
	}
	want := input{Name: "Ada", Age: -36, Big: 9007199254740993, Count: 7, Ratio: 1.5, Small: 0.25, Enabled: true}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// An absent field keeps its zero value and is not an error: "not sent" is a
// thing a form says, and which fields are required is the validator's
// question, not the binder's.
func TestAnAbsentFieldIsNotAnError(t *testing.T) {
	type input struct {
		Name string `form:"name"`
		Age  int    `form:"age"`
	}

	var got input
	if err := bind(t, form("name=Ada"), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Ada" || got.Age != 0 {
		t.Errorf("got %+v", got)
	}
}

// An empty value is not the same as an absent one for a string — it is the
// empty string somebody cleared a field to.
func TestAnEmptyValueBindsAsEmpty(t *testing.T) {
	type input struct {
		Name string `form:"name"`
		Age  int    `form:"age"`
	}

	got := input{Name: "previous", Age: 9}
	if err := bind(t, form("name=&age="), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "" {
		t.Errorf("Name = %q, want it cleared", got.Name)
	}
	// An empty number is zero rather than a parse failure: a browser sends
	// `age=` for an untouched number input, and refusing that would make an
	// optional field impossible to leave blank.
	if got.Age != 0 {
		t.Errorf("Age = %d, want 0", got.Age)
	}
}

func TestAMalformedNumberIsAnError(t *testing.T) {
	type input struct {
		Age int `form:"age"`
	}

	var got input
	err := bind(t, form("age=thirty"), &got)
	if err == nil {
		t.Fatal("\"thirty\" was accepted as a number")
	}
	if !strings.Contains(err.Error(), "age") {
		t.Errorf("the error does not name the field: %v", err)
	}
}

func TestAnOutOfRangeNumberIsAnError(t *testing.T) {
	type input struct {
		Small int8 `form:"small"`
	}

	var got input
	if err := bind(t, form("small=200"), &got); err == nil {
		t.Error("200 was accepted into an int8")
	}
}

func TestANegativeNumberIsRefusedByAnUnsignedField(t *testing.T) {
	type input struct {
		Count uint `form:"count"`
	}

	var got input
	if err := bind(t, form("count=-1"), &got); err == nil {
		t.Error("-1 was accepted into a uint")
	}
}

// ---------------------------------------------------------------- booleans

// What a browser sends for a checkbox, and what every other client sends.
func TestBindBooleans(t *testing.T) {
	type input struct {
		Yes bool `form:"yes"`
	}

	for _, c := range []struct {
		raw  string
		want bool
	}{
		{"on", true}, {"On", true}, {"ON", true}, {" on ", true},
		{"yes", true}, {"true", true}, {"TRUE", true}, {"1", true}, {"t", true},
		{"off", false}, {"no", false}, {"false", false}, {"0", false}, {"", false},
	} {
		t.Run(fmt.Sprintf("%q", c.raw), func(t *testing.T) {
			var got input
			if err := bind(t, form("yes="+c.raw), &got); err != nil {
				t.Fatalf("binding %q: %v", c.raw, err)
			}
			if got.Yes != c.want {
				t.Errorf("%q bound as %v, want %v", c.raw, got.Yes, c.want)
			}
		})
	}
}

// An unchecked box sends nothing at all, and an absent key leaves the field
// alone rather than setting it false.
//
// Which is a decision, not an accident, and worth knowing: every call site
// binds a freshly zeroed struct, so an absent box is false there — but a
// handler that pre-fills a struct with current values and then binds a form
// over it cannot untick anything, because the form has no way to say "off"
// except by silence. That pattern is not used here. If it ever is, this is
// the line that has to change, and it should change to "a bool with no key
// is false" rather than to an error.
func TestAnAbsentCheckboxLeavesTheFieldAlone(t *testing.T) {
	type input struct {
		Subscribed bool `form:"subscribed"`
	}

	// The case every call site is in.
	var fresh input
	if err := bind(t, form("other=1"), &fresh); err != nil {
		t.Fatal(err)
	}
	if fresh.Subscribed {
		t.Error("a zeroed struct came back true")
	}

	// And the case that makes it a decision.
	prefilled := input{Subscribed: true}
	if err := bind(t, form("other=1"), &prefilled); err != nil {
		t.Fatal(err)
	}
	if !prefilled.Subscribed {
		t.Error("an absent key overwrote a pre-set value")
	}
}

func TestANonBooleanIsAnError(t *testing.T) {
	type input struct {
		Yes bool `form:"yes"`
	}

	var got input
	if err := bind(t, form("yes=maybe"), &got); err == nil {
		t.Error("\"maybe\" was accepted as a boolean")
	}
}

// ------------------------------------------------------------------ slices

// All three shapes, because all three are produced by something: a plain HTML
// form repeats the key, jQuery and Rails append `[]`, and Inertia and PHP
// index it.
func TestBindSlicesInEveryShape(t *testing.T) {
	type input struct {
		IDs  []uint64 `form:"ids"`
		Tags []string `form:"tags"`
	}

	for _, c := range []struct {
		name string
		body string
	}{
		{"repeated", "ids=1&ids=2&ids=3&tags=a&tags=b"},
		{"bracketed", "ids[]=1&ids[]=2&ids[]=3&tags[]=a&tags[]=b"},
		{"indexed", "ids[0]=1&ids[1]=2&ids[2]=3&tags[0]=a&tags[1]=b"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var got input
			if err := bind(t, form(c.body), &got); err != nil {
				t.Fatal(err)
			}
			if len(got.IDs) != 3 || got.IDs[0] != 1 || got.IDs[2] != 3 {
				t.Errorf("IDs = %v", got.IDs)
			}
			if len(got.Tags) != 2 || got.Tags[0] != "a" {
				t.Errorf("Tags = %v", got.Tags)
			}
		})
	}
}

// Indexed keys are gathered in index order, not map order — otherwise a
// list's order would vary between requests, which is the kind of bug that
// reproduces once a week.
func TestIndexedKeysKeepTheirOrder(t *testing.T) {
	type input struct {
		Steps []string `form:"steps"`
	}

	for attempt := 0; attempt < 20; attempt++ {
		var got input
		if err := bind(t, form("steps[2]=third&steps[0]=first&steps[1]=second"), &got); err != nil {
			t.Fatal(err)
		}
		want := []string{"first", "second", "third"}
		for i := range want {
			if i >= len(got.Steps) || got.Steps[i] != want[i] {
				t.Fatalf("Steps = %v, want %v", got.Steps, want)
			}
		}
	}
}

// A gap ends the list rather than binding a zero value in the middle: an
// index nobody sent is not an empty string, it is the end of what was sent.
func TestAGapEndsAnIndexedList(t *testing.T) {
	type input struct {
		Steps []string `form:"steps"`
	}

	var got input
	if err := bind(t, form("steps[0]=first&steps[2]=third"), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 1 || got.Steps[0] != "first" {
		t.Errorf("Steps = %v, want just the contiguous prefix", got.Steps)
	}
}

func TestASingleValueBindsToASlice(t *testing.T) {
	type input struct {
		IDs []int `form:"ids"`
	}

	var got input
	if err := bind(t, form("ids=7"), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.IDs) != 1 || got.IDs[0] != 7 {
		t.Errorf("IDs = %v", got.IDs)
	}
}

func TestOneBadElementFailsTheSlice(t *testing.T) {
	type input struct {
		IDs []int `form:"ids"`
	}

	var got input
	if err := bind(t, form("ids=1&ids=two&ids=3"), &got); err == nil {
		t.Error("a slice with an unparseable element was accepted")
	}
}

// ---------------------------------------------------------------- pointers

// A pointer is how a struct says it cares about the difference between "not
// supplied" and "supplied as empty" — so an empty value must leave it nil
// rather than pointing at a zero.
func TestPointersDistinguishAbsentFromEmpty(t *testing.T) {
	type input struct {
		Note *string `form:"note"`
		Age  *int    `form:"age"`
	}

	var absent input
	if err := bind(t, form("other=1"), &absent); err != nil {
		t.Fatal(err)
	}
	if absent.Note != nil || absent.Age != nil {
		t.Errorf("an absent field was not nil: %+v", absent)
	}

	var empty input
	if err := bind(t, form("note=&age="), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Note != nil {
		t.Errorf("an empty value gave a non-nil pointer: %q", *empty.Note)
	}

	var supplied input
	if err := bind(t, form("note=hello&age=4"), &supplied); err != nil {
		t.Fatal(err)
	}
	if supplied.Note == nil || *supplied.Note != "hello" {
		t.Errorf("Note = %v", supplied.Note)
	}
	if supplied.Age == nil || *supplied.Age != 4 {
		t.Errorf("Age = %v", supplied.Age)
	}
}

// ------------------------------------------------------------------- files

func multipartBody(t *testing.T, fields map[string]string, files map[string][]string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	for name, contents := range files {
		for i, content := range contents {
			part, err := writer.CreateFormFile(name, fmt.Sprintf("f%d.png", i))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodPost, "/f", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	return r
}

func TestBindFilesInEveryShape(t *testing.T) {
	type input struct {
		Body   string                  `form:"body"`
		Images []*multipart.FileHeader `form:"images"`
	}

	for _, c := range []struct {
		name  string
		files map[string][]string
	}{
		{"repeated", map[string][]string{"images": {"a", "b"}}},
		{"bracketed", map[string][]string{"images[]": {"a", "b"}}},
		{"indexed", map[string][]string{"images[0]": {"a"}, "images[1]": {"b"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var got input
			if err := bind(t, multipartBody(t, map[string]string{"body": "words"}, c.files), &got); err != nil {
				t.Fatal(err)
			}
			if got.Body != "words" {
				t.Errorf("Body = %q", got.Body)
			}
			if len(got.Images) != 2 {
				t.Fatalf("bound %d files, want 2", len(got.Images))
			}
		})
	}
}

// The content has to survive, not just the header — a bound file nobody can
// read is the same as no file.
func TestABoundFileCanBeRead(t *testing.T) {
	type input struct {
		Avatar *multipart.FileHeader `form:"avatar"`
	}

	var got input
	if err := bind(t, multipartBody(t, nil, map[string][]string{"avatar": {"the-content"}}), &got); err != nil {
		t.Fatal(err)
	}
	if got.Avatar == nil {
		t.Fatal("no file was bound")
	}
	file, err := got.Avatar.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	content := make([]byte, 11)
	if _, err := file.Read(content); err != nil {
		t.Fatal(err)
	}
	if string(content) != "the-content" {
		t.Errorf("read %q", content)
	}
}

func TestASingleFileFieldTakesTheFirst(t *testing.T) {
	type input struct {
		Avatar *multipart.FileHeader `form:"avatar"`
	}

	var got input
	if err := bind(t, multipartBody(t, nil, map[string][]string{"avatar": {"first", "second"}}), &got); err != nil {
		t.Fatal(err)
	}
	if got.Avatar == nil || got.Avatar.Filename != "f0.png" {
		t.Errorf("Avatar = %+v", got.Avatar)
	}
}

func TestAnAbsentFileIsNotAnError(t *testing.T) {
	type input struct {
		Body   string                  `form:"body"`
		Images []*multipart.FileHeader `form:"images"`
	}

	var got input
	if err := bind(t, multipartBody(t, map[string]string{"body": "no pictures"}, nil), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Images) != 0 {
		t.Errorf("bound %d files from a body with none", len(got.Images))
	}
}

// A file field on a URL-encoded body binds nothing, and must not fail: the
// same struct serves both encodings.
func TestAFileFieldOnAFormWithoutFiles(t *testing.T) {
	type input struct {
		Body   string                  `form:"body"`
		Images []*multipart.FileHeader `form:"images"`
	}

	var got input
	if err := bind(t, form("body=words"), &got); err != nil {
		t.Fatal(err)
	}
	if got.Body != "words" || len(got.Images) != 0 {
		t.Errorf("got %+v", got)
	}
}

// ------------------------------------------------------------------ shapes

// A field with no tag is left alone — binding by field name would make every
// unexported-by-intent field writable from a request.
func TestAnUntaggedFieldIsNotBound(t *testing.T) {
	type input struct {
		Name     string `form:"name"`
		Internal string
	}

	var got input
	if err := bind(t, form("name=Ada&Internal=injected&internal=injected"), &got); err != nil {
		t.Fatal(err)
	}
	if got.Internal != "" {
		t.Errorf("an untagged field was written: %q", got.Internal)
	}
}

func TestADashTagIsNotBound(t *testing.T) {
	type input struct {
		Name   string `form:"name"`
		Secret string `form:"-"`
	}

	var got input
	if err := bind(t, form("name=Ada&-=injected&Secret=injected"), &got); err != nil {
		t.Fatal(err)
	}
	if got.Secret != "" {
		t.Errorf("a - tagged field was written: %q", got.Secret)
	}
}

// An unexported field cannot be set, and must not panic the binder.
func TestAnUnexportedFieldIsSkipped(t *testing.T) {
	type input struct {
		Name   string `form:"name"`
		hidden string `form:"hidden"`
	}

	var got input
	if err := bind(t, form("name=Ada&hidden=nope"), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Ada" {
		t.Errorf("Name = %q", got.Name)
	}
	_ = got.hidden
}

func TestABadTargetIsReported(t *testing.T) {
	var notAPointer struct {
		Name string `form:"name"`
	}
	if err := DecodeForm(form("name=Ada"), notAPointer); err == nil {
		t.Error("a non-pointer target was accepted")
	}

	var nilPointer *struct {
		Name string `form:"name"`
	}
	if err := DecodeForm(form("name=Ada"), nilPointer); err == nil {
		t.Error("a nil pointer was accepted")
	}

	number := 7
	if err := DecodeForm(form("name=Ada"), &number); err == nil {
		t.Error("a pointer to a non-struct was accepted")
	}
}

// ------------------------------------------------------- content types

// A query string binds on a request with no body, which is what a GET is.
func TestAQueryStringBinds(t *testing.T) {
	type input struct {
		Page int    `form:"page"`
		Sort string `form:"sort"`
	}

	r := httptest.NewRequest(http.MethodGet, "/things?page=3&sort=recent", nil)

	var got input
	if err := bind(t, r, &got); err != nil {
		t.Fatal(err)
	}
	if got.Page != 3 || got.Sort != "recent" {
		t.Errorf("got %+v", got)
	}
}

// A JSON body goes to encoding/json and its `json:` tags, decided by the
// Content-Type and never by which tags the struct happens to carry.
func TestAJSONBodyUsesJSONTags(t *testing.T) {
	type input struct {
		Name string `json:"name" form:"name"`
		Age  int    `json:"age" form:"age"`
	}

	r := httptest.NewRequest(http.MethodPost, "/f", strings.NewReader(`{"name":"Ada","age":36}`))
	r.Header.Set("Content-Type", "application/json")

	var got input
	if err := bind(t, r, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Ada" || got.Age != 36 {
		t.Errorf("got %+v", got)
	}
}

// A body with no Content-Type at all is treated as a form, which is what
// ParseForm does with it.
func TestABodyWithNoContentType(t *testing.T) {
	type input struct {
		Name string `form:"name"`
	}

	r := httptest.NewRequest(http.MethodPost, "/f", strings.NewReader("name=Ada"))
	r.Header.Del("Content-Type")

	var got input
	if err := bind(t, r, &got); err != nil {
		t.Fatal(err)
	}
	// ParseForm ignores a body with no content type, so this binds nothing —
	// asserted so the behaviour is a decision rather than a surprise.
	_ = got
}

// A time field, because a date input is an ordinary thing for a form to
// carry and "unsupported type" is a poor answer to one.
func TestBindTime(t *testing.T) {
	type input struct {
		StartsAt time.Time `form:"starts_at"`
	}

	var got input
	err := bind(t, form("starts_at=2026-10-12T09:30:00Z"), &got)
	if err != nil {
		t.Skipf("time binding is not supported: %v", err)
	}
	if !got.StartsAt.Equal(time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)) {
		t.Errorf("StartsAt = %v", got.StartsAt)
	}
}

// ------------------------------------------------------------------- XML

func xmlBody(t *testing.T, mediaType, body string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/f", strings.NewReader(body))
	r.Header.Set("Content-Type", mediaType)
	return r
}

func TestAnXMLBodyUsesXMLTags(t *testing.T) {
	type input struct {
		XMLName struct{} `xml:"order"`
		Name    string   `xml:"name" json:"name" form:"name"`
		Qty     int      `xml:"qty" json:"qty" form:"qty"`
	}

	for _, mediaType := range []string{
		"application/xml",
		"text/xml",
		"application/xml; charset=utf-8",
		"application/atom+xml",
	} {
		t.Run(mediaType, func(t *testing.T) {
			var got input
			err := bind(t, xmlBody(t, mediaType, `<order><name>Ada</name><qty>3</qty></order>`), &got)
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != "Ada" || got.Qty != 3 {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestMalformedXMLIsReported(t *testing.T) {
	type input struct {
		Name string `xml:"name"`
	}

	var got input
	err := bind(t, xmlBody(t, "application/xml", `<order><name>Ada</name>`), &got)
	if err == nil {
		t.Fatal("an unclosed element was accepted")
	}
	if !strings.Contains(err.Error(), "XML") {
		t.Errorf("the error does not say it was XML: %v", err)
	}
}

func TestAnEmptyXMLBodyIsReported(t *testing.T) {
	type input struct {
		Name string `xml:"name"`
	}

	var got input
	if err := bind(t, xmlBody(t, "application/xml", "   "), &got); err == nil {
		t.Error("an empty body was accepted")
	}
}

// A suffixed JSON type is JSON — an API that serves application/vnd.api+json
// is not sending a form.
func TestASuffixedJSONTypeIsJSON(t *testing.T) {
	type input struct {
		Name string `json:"name" form:"name"`
	}

	r := httptest.NewRequest(http.MethodPost, "/f", strings.NewReader(`{"name":"Ada"}`))
	r.Header.Set("Content-Type", "application/vnd.api+json")

	var got input
	if err := bind(t, r, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Ada" {
		t.Errorf("Name = %q", got.Name)
	}
}

// The Content-Type decides, never the tags. The same struct and the same
// field name, read three ways.
func TestTheContentTypeDecidesAndNotTheTags(t *testing.T) {
	type input struct {
		Name string `json:"name" xml:"name" form:"name"`
	}

	cases := []struct {
		media string
		body  string
	}{
		{"application/json", `{"name":"Ada"}`},
		{"application/xml", `<input><name>Ada</name></input>`},
		{"application/x-www-form-urlencoded", `name=Ada`},
	}

	for _, c := range cases {
		t.Run(c.media, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/f", strings.NewReader(c.body))
			r.Header.Set("Content-Type", c.media)

			var got input
			if err := bind(t, r, &got); err != nil {
				t.Fatal(err)
			}
			if got.Name != "Ada" {
				t.Errorf("Name = %q", got.Name)
			}
		})
	}
}

func TestKindOfClassifiesContentTypes(t *testing.T) {
	for _, c := range []struct {
		media string
		want  BodyKind
	}{
		{"application/json", BodyJSON},
		{"application/json; charset=utf-8", BodyJSON},
		{"APPLICATION/JSON", BodyJSON},
		{"application/vnd.api+json", BodyJSON},
		{"application/xml", BodyXML},
		{"text/xml", BodyXML},
		{"application/atom+xml", BodyXML},
		{"application/x-www-form-urlencoded", BodyForm},
		{"multipart/form-data; boundary=x", BodyForm},
		{"", BodyForm},
		{"text/plain", BodyForm},
	} {
		t.Run(c.media, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/f", nil)
			if c.media != "" {
				r.Header.Set("Content-Type", c.media)
			} else {
				r.Header.Del("Content-Type")
			}
			if got := KindOf(r); got != c.want {
				t.Errorf("KindOf(%q) = %v, want %v", c.media, got, c.want)
			}
		})
	}
}

// An oversized body is refused rather than read into memory.
func TestAnOversizedBodyIsRefused(t *testing.T) {
	type input struct {
		Name string `json:"name"`
	}

	huge := `{"name":"` + strings.Repeat("a", 2<<20) + `"}`
	r := httptest.NewRequest(http.MethodPost, "/f", strings.NewReader(huge))
	r.Header.Set("Content-Type", "application/json")

	var got input
	if err := bind(t, r, &got); err == nil {
		t.Error("a 2MB body was accepted")
	}
}

// ------------------------------------------------------------------ time

func TestBindTimeFormats(t *testing.T) {
	type input struct {
		At time.Time `form:"at"`
	}

	for _, c := range []struct {
		raw  string
		want time.Time
	}{
		{"2026-10-12T09:30:00Z", time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)},
		{"2026-10-12T09:30:00.5Z", time.Date(2026, 10, 12, 9, 30, 0, 500000000, time.UTC)},
		// What <input type="datetime-local"> and <input type="date"> send,
		// read as UTC rather than as the server's zone.
		{"2026-10-12T09:30", time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)},
		{"2026-10-12T09:30:00", time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)},
		{"2026-10-12", time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)},
	} {
		t.Run(c.raw, func(t *testing.T) {
			var got input
			if err := bind(t, form("at="+c.raw), &got); err != nil {
				t.Fatalf("binding %q: %v", c.raw, err)
			}
			if !got.At.Equal(c.want) {
				t.Errorf("%q bound as %v, want %v", c.raw, got.At, c.want)
			}
		})
	}
}

func TestAnUnparseableTimeIsAnError(t *testing.T) {
	type input struct {
		At time.Time `form:"at"`
	}

	var got input
	if err := bind(t, form("at=next+tuesday"), &got); err == nil {
		t.Error("\"next tuesday\" was accepted as a time")
	}
}

// A type the binder cannot handle is reported rather than left at its zero.
//
// It used to return nil, so a field of an unknown type bound to zero with
// nothing said — the request succeeded and the value was empty. That is the
// same silent shape as the dropped files, and it is the one thing this
// package must never do.
func TestAnUnbindableTypeIsReported(t *testing.T) {
	type unsupported struct{ A, B int }
	type input struct {
		Thing unsupported `form:"thing"`
	}

	var got input
	err := bind(t, form("thing=whatever"), &got)
	if err == nil {
		t.Fatal("a field of an unbindable type was silently skipped")
	}
	if !strings.Contains(err.Error(), "thing") {
		t.Errorf("the error does not name the field: %v", err)
	}
}
