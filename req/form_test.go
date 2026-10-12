package req

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type formResponder struct {
	r *http.Request
	w http.ResponseWriter
}

func (f *formResponder) Request() *http.Request              { return f.r }
func (f *formResponder) ResponseWriter() http.ResponseWriter { return f.w }

type formInput struct {
	Name     string   `json:"name" form:"name"`
	Tagline  string   `json:"tagline" form:"tagline"`
	Agree    bool     `json:"agree" form:"agree"`
	Count    int      `json:"count" form:"count"`
	Price    float64  `json:"price" form:"price"`
	TopicIDs []uint64 `json:"topic_ids" form:"topic_ids"`
	ParentID *uint64  `json:"parent_id" form:"parent_id"`
	Ignored  string   `json:"ignored" form:"-"`
	Untagged string
	Avatar   *multipart.FileHeader `form:"avatar"`
}

func post(body, contentType string) *formResponder {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	return &formResponder{r: r, w: httptest.NewRecorder()}
}

const urlencoded = "application/x-www-form-urlencoded"

// The regression this file exists for: a struct tagged `form:` bound nothing,
// because httpin reads only its own `in:` tag. A plain HTML form arrived empty
// and validation reported "required" for every field the person had filled in.
func TestAFormBodyBindsByFormTag(t *testing.T) {
	rr := post("name=Sidekick&tagline=A+small+helper&agree=on&count=3&price=9.99", urlencoded)

	var input formInput
	if err := ParseInput(rr, &input); err != nil {
		t.Fatal(err)
	}

	if input.Name != "Sidekick" {
		t.Errorf("Name = %q", input.Name)
	}
	if input.Tagline != "A small helper" {
		t.Errorf("Tagline = %q", input.Tagline)
	}
	if !input.Agree {
		t.Error(`Agree = false for a checkbox sending "on"`)
	}
	if input.Count != 3 {
		t.Errorf("Count = %d", input.Count)
	}
	if input.Price != 9.99 {
		t.Errorf("Price = %v", input.Price)
	}
}

// Both encodings a list arrives in, and in index order rather than map order.
func TestAFormBindsListsInEitherEncoding(t *testing.T) {
	cases := map[string]string{
		"repeated": "topic_ids=7&topic_ids=8&topic_ids=9",
		"indexed":  "topic_ids[0]=7&topic_ids[1]=8&topic_ids[2]=9",
		"bracket":  "topic_ids[]=7&topic_ids[]=8&topic_ids[]=9",
	}
	for name, body := range cases {
		var input formInput
		if err := ParseInput(post(body, urlencoded), &input); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := []uint64{7, 8, 9}
		if len(input.TopicIDs) != len(want) {
			t.Fatalf("%s: got %v", name, input.TopicIDs)
		}
		for i := range want {
			if input.TopicIDs[i] != want[i] {
				t.Fatalf("%s: got %v, want %v", name, input.TopicIDs, want)
			}
		}
	}
}

// An optional field distinguishes "not supplied" from "supplied as empty",
// which is the only reason it is a pointer.
func TestAnOptionalFieldStaysNilWhenAbsentOrEmpty(t *testing.T) {
	for name, body := range map[string]string{
		"absent": "name=Sidekick",
		"empty":  "name=Sidekick&parent_id=",
	} {
		var input formInput
		if err := ParseInput(post(body, urlencoded), &input); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if input.ParentID != nil {
			t.Errorf("%s: ParentID = %v, want nil", name, *input.ParentID)
		}
	}

	var input formInput
	if err := ParseInput(post("parent_id=42", urlencoded), &input); err != nil {
		t.Fatal(err)
	}
	if input.ParentID == nil || *input.ParentID != 42 {
		t.Errorf("ParentID = %v, want 42", input.ParentID)
	}
}

// An unchecked checkbox sends nothing, so a missing key is false rather than an
// error — otherwise every form with a checkbox would fail to submit.
func TestAMissingCheckboxIsFalseNotAnError(t *testing.T) {
	var input formInput
	input.Agree = true
	if err := ParseInput(post("name=Sidekick", urlencoded), &input); err != nil {
		t.Fatal(err)
	}
	// Absent means untouched, which is what lets a handler pre-fill a struct.
	if !input.Agree {
		t.Error("an absent key overwrote a pre-set value")
	}

	var fresh formInput
	if err := ParseInput(post("name=Sidekick&agree=off", urlencoded), &fresh); err != nil {
		t.Fatal(err)
	}
	if fresh.Agree {
		t.Error(`agree=off bound true`)
	}
}

func TestAFormRefusesAValueOfTheWrongShape(t *testing.T) {
	for _, body := range []string{"count=three", "price=free", "topic_ids=abc"} {
		var input formInput
		if err := ParseInput(post(body, urlencoded), &input); err == nil {
			t.Errorf("%q bound without complaint", body)
		}
	}
}

func TestAFormSkipsUntaggedAndExcludedFields(t *testing.T) {
	var input formInput
	if err := ParseInput(post("Ignored=x&ignored=x&Untagged=y", urlencoded), &input); err != nil {
		t.Fatal(err)
	}
	if input.Ignored != "" {
		t.Errorf(`a field tagged form:"-" was bound: %q`, input.Ignored)
	}
	if input.Untagged != "" {
		t.Errorf("an untagged field was bound: %q", input.Untagged)
	}
}

// Multipart carries both text fields and files, and the file field must not be
// parsed as text — that would fail every upload.
func TestAMultipartFormBindsTextAndLeavesFilesAlone(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("name", "Sidekick")
	_ = writer.WriteField("count", "2")
	part, err := writer.CreateFormFile("avatar", "avatar.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("\x89PNG\r\n\x1a\n"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodPost, "/", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())

	var input formInput
	if err := ParseInput(&formResponder{r: r, w: httptest.NewRecorder()}, &input); err != nil {
		t.Fatal(err)
	}
	if input.Name != "Sidekick" || input.Count != 2 {
		t.Errorf("bound %+v", input)
	}
	if _, _, err := r.FormFile("avatar"); err != nil {
		t.Errorf("the uploaded file is no longer readable: %v", err)
	}
}

// A JSON body is untouched by any of this.
func TestAJSONBodyStillBindsByJSONTag(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"Sidekick","count":5}`))
	r.Header.Set("Content-Type", "application/json")

	var input formInput
	if err := ParseInput(&formResponder{r: r, w: httptest.NewRecorder()}, &input); err != nil {
		t.Fatal(err)
	}
	if input.Name != "Sidekick" || input.Count != 5 {
		t.Errorf("bound %+v", input)
	}
}

// A struct tagged for httpin keeps using httpin, so adding this decoder changed
// nothing for inputs that were already working.
func TestAnHTTPInTaggedStructStillUsesHTTPIn(t *testing.T) {
	type httpinInput struct {
		Name string `in:"form=name"`
	}

	var input httpinInput
	if err := ParseInput(post("name=Sidekick", urlencoded), &input); err != nil {
		t.Fatal(err)
	}
	if input.Name != "Sidekick" {
		t.Errorf("Name = %q", input.Name)
	}
	if HasFormTags(&input) {
		t.Error("an in-tagged struct was reported as form-tagged")
	}
}

// buildUpload makes a multipart request with files under the given field
// names, which is how a browser sends them.
func buildUpload(t *testing.T, fields map[string]string, files map[string][]string) *http.Request {
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
			part, err := writer.CreateFormFile(name, fmt.Sprintf("file-%d.png", i))
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

	request := httptest.NewRequest(http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

// TestDecodeFormBindsUploadedFiles.
//
// DecodeForm used to skip every file field, so a struct declaring
// `Images []*multipart.FileHeader` was handed an empty slice on every request.
// The damage was that it looks like success: a handler that writes
// `if len(input.Images) == 0 { return nil, nil }` accepts the upload, answers
// 302, attaches nothing, and says nothing. The first report of it was somebody
// posting a picture and getting a post without one.
func TestDecodeFormBindsUploadedFiles(t *testing.T) {
	type input struct {
		Body   string                  `form:"body"`
		Images []*multipart.FileHeader `form:"images"`
	}

	t.Run("repeated keys", func(t *testing.T) {
		var got input
		request := buildUpload(t,
			map[string]string{"body": "look at this"},
			map[string][]string{"images": {"first", "second"}})

		if err := DecodeForm(request, &got); err != nil {
			t.Fatal(err)
		}
		if got.Body != "look at this" {
			t.Errorf("Body = %q", got.Body)
		}
		if len(got.Images) != 2 {
			t.Fatalf("bound %d files, want 2", len(got.Images))
		}
		if got.Images[0].Filename != "file-0.png" {
			t.Errorf("first file = %q", got.Images[0].Filename)
		}
	})

	// What an Inertia client sends: one key per file, indexed.
	t.Run("indexed keys", func(t *testing.T) {
		var got input
		request := buildUpload(t,
			map[string]string{"body": "look at this"},
			map[string][]string{"images[0]": {"first"}, "images[1]": {"second"}})

		if err := DecodeForm(request, &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Images) != 2 {
			t.Fatalf("bound %d files, want 2", len(got.Images))
		}
	})

	t.Run("bracketed keys", func(t *testing.T) {
		var got input
		request := buildUpload(t, nil, map[string][]string{"images[]": {"only"}})

		if err := DecodeForm(request, &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Images) != 1 {
			t.Fatalf("bound %d files, want 1", len(got.Images))
		}
	})

	// An absent file is not an error: a field may simply not be filled in.
	t.Run("no files", func(t *testing.T) {
		var got input
		request := buildUpload(t, map[string]string{"body": "words only"}, nil)

		if err := DecodeForm(request, &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Images) != 0 {
			t.Errorf("bound %d files, want none", len(got.Images))
		}
		if got.Body != "words only" {
			t.Errorf("Body = %q", got.Body)
		}
	})
}

// A single file field takes the first one, which is what one header can mean.
func TestDecodeFormBindsASingleFile(t *testing.T) {
	type input struct {
		Avatar *multipart.FileHeader `form:"avatar"`
	}

	var got input
	request := buildUpload(t, nil, map[string][]string{"avatar": {"portrait"}})
	if err := DecodeForm(request, &got); err != nil {
		t.Fatal(err)
	}
	if got.Avatar == nil {
		t.Fatal("no file was bound")
	}
	if got.Avatar.Filename != "file-0.png" {
		t.Errorf("Filename = %q", got.Avatar.Filename)
	}
}

// A URL-encoded body has no files, and asking for them must not fail.
func TestDecodeFormWithFilesOnANonMultipartBody(t *testing.T) {
	type input struct {
		Body   string                  `form:"body"`
		Images []*multipart.FileHeader `form:"images"`
	}

	request := httptest.NewRequest(http.MethodPost, "/upload",
		strings.NewReader("body=words+only"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var got input
	if err := DecodeForm(request, &got); err != nil {
		t.Fatal(err)
	}
	if got.Body != "words only" {
		t.Errorf("Body = %q", got.Body)
	}
	if len(got.Images) != 0 {
		t.Errorf("bound %d files from a form with none", len(got.Images))
	}
}

// TestHttpinBindsACheckbox.
//
// A checked box sends `on`, and strconv.ParseBool knows neither that nor
// `yes` — so httpin refused a form any browser produces, with
// `strconv.ParseBool: parsing "on": invalid syntax`, and the request became
// a 500. DecodeForm has always accepted them, which is the real problem: two
// binders in one framework disagreeing about what `on` means, with a struct
// tag deciding which one a given input gets.
func TestHttpinBindsACheckbox(t *testing.T) {
	type input struct {
		Subscribed bool `in:"form=subscribed"`
		Notify     bool `in:"form=notify"`
	}

	for _, c := range []struct {
		sends string
		want  bool
	}{
		{sends: "on", want: true},   // a checked box
		{sends: "yes", want: true},  // some clients
		{sends: "true", want: true}, // JSON-ish clients and our own composer
		{sends: "1", want: true},
		{sends: "off", want: false},
		{sends: "no", want: false},
		{sends: "false", want: false},
		{sends: "0", want: false},
	} {
		t.Run(c.sends, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/f",
				strings.NewReader("subscribed="+c.sends))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			var got input
			rr := &formResponder{r: request, w: httptest.NewRecorder()}
			if err := ParseInput(rr, &got); err != nil {
				t.Fatalf("binding %q: %v", c.sends, err)
			}
			if got.Subscribed != c.want {
				t.Errorf("subscribed=%s bound as %v, want %v", c.sends, got.Subscribed, c.want)
			}
			// An unchecked box sends nothing at all, which must stay false
			// rather than becoming an error.
			if got.Notify {
				t.Error("an absent checkbox bound as true")
			}
		})
	}
}

// A value that is not a boolean at all is still an error, rather than being
// quietly read as false.
func TestHttpinRefusesANonBoolean(t *testing.T) {
	type input struct {
		Subscribed bool `in:"form=subscribed"`
	}

	request := httptest.NewRequest(http.MethodPost, "/f",
		strings.NewReader("subscribed=maybe"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var got input
	rr := &formResponder{r: request, w: httptest.NewRecorder()}
	if err := ParseInput(rr, &got); err == nil {
		t.Error("\"maybe\" was accepted as a boolean")
	}
}
