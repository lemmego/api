package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"strings"
	"testing"
)

// uploadField builds a *multipart.FileHeader the way a real request would, so
// the rules are exercised against the type they actually meet rather than a
// stand-in.
func uploadField(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("avatar", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	reader := multipart.NewReader(&body, writer.Boundary())
	form, err := reader.ReadForm(10 << 20)
	if err != nil {
		t.Fatal(err)
	}
	return form.File["avatar"][0]
}

func pngBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The existing Dimensions and MimeTypes rules assert the value to a string
// and os.Open it, so handing them an upload silently does nothing: the
// assertion fails and the field comes back unchanged. A form that looked
// validated was not. This pins the behaviour that replaced them.
func TestTheOldPathRulesSilentlyIgnoreAnUpload(t *testing.T) {
	header := uploadField(t, "avatar.png", pngBytes(t, 10, 10))

	v := NewValidator()
	v.Field("avatar", header).MimeTypes([]string{"image/png"}).Dimensions(10, 10)

	if !v.IsValid() {
		t.Fatalf("unexpectedly reported errors: %v", v.Errors)
	}
	// The point is that they reported nothing either way — they never ran.
	// Which is why FileTypes and ImageDimensions exist.
}

func TestFileTypesSniffsTheContent(t *testing.T) {
	image := uploadField(t, "avatar.png", pngBytes(t, 8, 8))

	v := NewValidator()
	v.Field("avatar", image).FileTypes("image/png", "image/jpeg")
	if !v.IsValid() {
		t.Errorf("a real PNG was rejected: %v", v.Errors)
	}

	v = NewValidator()
	v.Field("avatar", image).FileTypes("image/gif")
	if v.IsValid() {
		t.Error("a PNG passed a GIF-only rule")
	}
}

// A .png extension on an HTML file is how a stored upload becomes a
// cross-site scripting vector once it is served from the same origin. The
// filename and the browser's Content-Type are both client-supplied, so
// neither can be trusted.
func TestFileTypesIgnoresTheFilename(t *testing.T) {
	disguised := uploadField(t, "avatar.png", []byte(`<html><script>alert(1)</script></html>`))

	v := NewValidator()
	v.Field("avatar", disguised).FileTypes("image/png")
	if v.IsValid() {
		t.Error("an HTML file named .png passed an image-only rule")
	}
}

func TestImageRejectsSomethingNamedLikeAnImage(t *testing.T) {
	v := NewValidator()
	v.Field("avatar", uploadField(t, "avatar.png", []byte("not an image at all"))).Image()
	if v.IsValid() {
		t.Error("a text file passed the Image rule")
	}

	v = NewValidator()
	v.Field("avatar", uploadField(t, "avatar.png", pngBytes(t, 4, 4))).Image()
	if !v.IsValid() {
		t.Errorf("a real PNG was rejected: %v — are the image decoders registered?", v.Errors)
	}
}

func TestImageDimensions(t *testing.T) {
	small := uploadField(t, "a.png", pngBytes(t, 16, 16))
	large := uploadField(t, "b.png", pngBytes(t, 4000, 4000))

	v := NewValidator()
	v.Field("avatar", small).ImageDimensions(64, 64, 0, 0)
	if v.IsValid() {
		t.Error("a 16x16 image passed a 64x64 minimum")
	}

	v = NewValidator()
	v.Field("avatar", large).ImageDimensions(0, 0, 2048, 2048)
	if v.IsValid() {
		t.Error("a 4000x4000 image passed a 2048x2048 maximum")
	}

	v = NewValidator()
	v.Field("avatar", small).ImageDimensions(8, 8, 32, 32)
	if !v.IsValid() {
		t.Errorf("a 16x16 image failed an 8–32 range: %v", v.Errors)
	}
}

func TestMaxAndMinSize(t *testing.T) {
	header := uploadField(t, "a.png", bytes.Repeat([]byte("x"), 2048))

	v := NewValidator()
	v.Field("avatar", header).MaxSize(1024)
	if v.IsValid() {
		t.Error("a 2KB file passed a 1KB maximum")
	}
	if !strings.Contains(strings.Join(v.Errors["avatar"], " "), "1KB") {
		t.Errorf("the message does not name the limit: %v", v.Errors)
	}

	v = NewValidator()
	v.Field("avatar", header).MinSize(4096)
	if v.IsValid() {
		t.Error("a 2KB file passed a 4KB minimum")
	}

	v = NewValidator()
	v.Field("avatar", header).MaxSize(4096).MinSize(1024)
	if !v.IsValid() {
		t.Errorf("a 2KB file failed a 1–4KB range: %v", v.Errors)
	}
}

// Reading to sniff must not consume the file: a later rule and the handler
// both need the whole thing.
func TestSniffingDoesNotConsumeTheUpload(t *testing.T) {
	content := pngBytes(t, 12, 12)
	header := uploadField(t, "a.png", content)

	v := NewValidator()
	v.Field("avatar", header).FileTypes("image/png").Image().ImageDimensions(8, 8, 64, 64)
	if !v.IsValid() {
		t.Fatalf("chained rules failed, so one consumed the file: %v", v.Errors)
	}

	// And the handler can still read it in full.
	file, err := header.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var read bytes.Buffer
	if _, err := read.ReadFrom(file); err != nil {
		t.Fatal(err)
	}
	if read.Len() != len(content) {
		t.Errorf("the handler read %d bytes of %d", read.Len(), len(content))
	}
}

// A field that is not an upload is left alone, because Required is what
// decides whether an absent file is acceptable. Failing here too would report
// two errors for one missing field.
func TestUploadRulesIgnoreANonUpload(t *testing.T) {
	v := NewValidator()
	v.Field("avatar", nil).MaxSize(10).Image().FileTypes("image/png")
	if !v.IsValid() {
		t.Errorf("a nil value produced upload errors: %v", v.Errors)
	}

	v = NewValidator()
	v.Field("avatar", "/some/path.png").Image()
	if !v.IsValid() {
		t.Errorf("a string value produced upload errors: %v", v.Errors)
	}
}

// Check is the seam a database-backed rule needs: a lookup that fails must be
// distinguishable from a value that is invalid, so a handler can answer 503
// rather than telling someone their handle is taken when the database is
// simply unreachable.
func TestCheckSeparatesAFailedRuleFromAnInvalidValue(t *testing.T) {
	boom := errOf("database is unreachable")

	v := NewValidator()
	v.Field("handle", "ada").Check(func(any) (bool, string, error) {
		return false, "could not check", boom
	})

	if v.IsValid() {
		t.Error("a failed rule reported valid")
	}
	if v.Err() == nil {
		t.Fatal("Err() is nil, so a handler cannot tell this from a validation failure")
	}
	if v.Err().Error() != "database is unreachable" {
		t.Errorf("Err() = %v", v.Err())
	}

	// An ordinary invalid value records a message and no Err.
	v = NewValidator()
	v.Field("handle", "ada").Check(func(any) (bool, string, error) {
		return false, "already taken", nil
	})
	if v.IsValid() {
		t.Error("an invalid value reported valid")
	}
	if v.Err() != nil {
		t.Errorf("Err() = %v for an ordinary validation failure, want nil", v.Err())
	}

	// And a passing rule records neither.
	v = NewValidator()
	v.Field("handle", "ada").Check(func(any) (bool, string, error) { return true, "", nil })
	if !v.IsValid() || v.Err() != nil {
		t.Errorf("a passing rule recorded %v / %v", v.Errors, v.Err())
	}
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func errOf(s string) error { return simpleError(s) }
