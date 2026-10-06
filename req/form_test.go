package req

import (
	"bytes"
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
