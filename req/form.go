package req

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// maxMultipartMemory is what is held in memory before a part spills to disk.
const maxMultipartMemory = 32 << 20 // 32 MiB

// HasFormTags reports whether a struct declares any field this package can
// bind, under either tag vocabulary.
//
// It no longer decides which decoder runs — the Content-Type does — and is
// kept because a caller may reasonably want to know whether a struct is
// bindable at all before handing it a request.
// FieldName returns the form field a struct field binds to, from either tag
// vocabulary, and whether it binds at all.
//
// Two vocabularies exist because two binders did: `form:"name"` is what this
// package reads and what `lemmego g input` used to generate; `in:"form=name"`
// is httpin's, which the generator emitted for a while. Supporting both here
// is what let httpin be removed without a sweep of every project — and the
// reason it had to go is that the two binders disagreed in ways nobody could
// see from a struct: on which array shapes bind, on whether a checkbox's
// `on` is true, and on whether files bind at all. A single struct tag decided
// which set of answers an input got.
//
// `-` means "never bind this", as it does for encoding/json.
func FieldName(field reflect.StructField) (string, bool) {
	if tag := field.Tag.Get("form"); tag != "" {
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			return "", false
		}
		return name, true
	}
	// httpin's form directive: `in:"form=name"`, possibly among others
	// separated by ";", and possibly with alternatives separated by ",".
	for _, directive := range strings.Split(field.Tag.Get("in"), ";") {
		directive = strings.TrimSpace(directive)
		if !strings.HasPrefix(directive, "form=") {
			continue
		}
		name := strings.Split(strings.TrimPrefix(directive, "form="), ",")[0]
		if name == "" || name == "-" {
			return "", false
		}
		return name, true
	}
	return "", false
}

func HasFormTags(inputStruct any) bool {
	value := reflect.ValueOf(inputStruct)
	for value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return false
	}

	t := value.Type()
	for i := 0; i < t.NumField(); i++ {
		if _, ok := FieldName(t.Field(i)); ok {
			return true
		}
	}
	return false
}

// DecodeForm binds a URL-encoded or multipart form onto a struct by its `form:`
// tags.
//
// It reads `form:"name"` and httpin's `in:"form=name"` alike — see FieldName
// — so an input written for either convention binds the same way.
//
// Arrays are accepted in all three shapes a browser or an HTTP client
// produces: repeated keys (`topic_ids=1&topic_ids=2`), indexed keys
// (`topic_ids[0]=1&topic_ids[1]=2`) and bracketed keys (`topic_ids[]=1`).
// Files too, under the same three.
func DecodeForm(r *http.Request, inputStruct any) error {
	values, err := formValues(r)
	if err != nil {
		return err
	}

	target := reflect.ValueOf(inputStruct)
	if target.Kind() != reflect.Ptr || target.IsNil() {
		return fmt.Errorf("req: DecodeForm needs a non-nil pointer, got %T", inputStruct)
	}
	target = target.Elem()
	if target.Kind() != reflect.Struct {
		return fmt.Errorf("req: DecodeForm needs a pointer to a struct, got %T", inputStruct)
	}

	t := target.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, binds := FieldName(field)
		if !binds || !target.Field(i).CanSet() {
			continue
		}
		// A file is not a form value: it lives in MultipartForm.File, not in
		// the value map, so it is bound from there.
		//
		// This used to `continue`, on the grounds that a file is read through
		// FormFile later. The effect was that a struct declaring
		// `Images []*multipart.FileHeader` was handed an empty slice on every
		// request — and a handler that reasonably writes
		// `if len(input.Images) == 0 { return nil, nil }` then accepted the
		// upload, returned success, and attached nothing. Silent, and on the
		// happy path.
		if isFileField(field.Type) {
			if err := setFileField(r, target.Field(i), name); err != nil {
				return fmt.Errorf("req: %s: %w", name, err)
			}
			continue
		}

		raw, ok := lookup(values, name)
		if !ok {
			continue
		}
		if err := setField(target.Field(i), raw); err != nil {
			return fmt.Errorf("req: %s: %w", name, err)
		}
	}
	return nil
}

// formValues parses whichever form the request carries.
func formValues(r *http.Request) (map[string][]string, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if r.MultipartForm == nil {
			if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
				return nil, err
			}
		}
		return r.Form, nil
	}
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	return r.Form, nil
}

// lookup finds a field's values under either encoding of a list.
func lookup(values map[string][]string, name string) ([]string, bool) {
	if found, ok := values[name]; ok {
		return found, true
	}
	// Indexed keys, gathered in index order rather than map order — otherwise
	// a list's order would vary between requests.
	var indexed []string
	for index := 0; ; index++ {
		found, ok := values[fmt.Sprintf("%s[%d]", name, index)]
		if !ok || len(found) == 0 {
			break
		}
		indexed = append(indexed, found[0])
	}
	if len(indexed) > 0 {
		return indexed, true
	}
	// "name[]" is what some clients send for a repeated field.
	if found, ok := values[name+"[]"]; ok {
		return found, true
	}
	return nil, false
}

// setFileField binds the uploaded headers for one field.
//
// It accepts the same three shapes as lookup does for values, because a client
// that sends `images[0]` for its text fields sends `images[0]` for its files
// too, and a form with a repeated input sends `images` several times. A field
// that is not a slice takes the first file and ignores the rest, which is what
// a single `*multipart.FileHeader` can mean.
func setFileField(r *http.Request, field reflect.Value, name string) error {
	if r.MultipartForm == nil {
		// Not a multipart request, or one nothing has parsed. Either way there
		// are no files, and an absent file is not an error — a field may
		// simply not have been filled in.
		return nil
	}

	headers := lookupFiles(r.MultipartForm.File, name)
	if len(headers) == 0 {
		return nil
	}

	if field.Kind() == reflect.Slice {
		slice := reflect.MakeSlice(field.Type(), 0, len(headers))
		for _, header := range headers {
			value, err := fileValue(field.Type().Elem(), header)
			if err != nil {
				return err
			}
			slice = reflect.Append(slice, value)
		}
		field.Set(slice)
		return nil
	}

	value, err := fileValue(field.Type(), headers[0])
	if err != nil {
		return err
	}
	field.Set(value)
	return nil
}

// fileValue adapts a header to whichever of *multipart.FileHeader or
// multipart.FileHeader the struct declared.
func fileValue(t reflect.Type, header *multipart.FileHeader) (reflect.Value, error) {
	switch {
	case t == reflect.TypeOf(header):
		return reflect.ValueOf(header), nil
	case t == reflect.TypeOf(*header):
		return reflect.ValueOf(*header), nil
	default:
		return reflect.Value{}, fmt.Errorf("cannot bind a file to %s", t)
	}
}

// lookupFiles is lookup, over the file map.
func lookupFiles(files map[string][]*multipart.FileHeader, name string) []*multipart.FileHeader {
	if found, ok := files[name]; ok && len(found) > 0 {
		return found
	}
	var indexed []*multipart.FileHeader
	for index := 0; ; index++ {
		found, ok := files[fmt.Sprintf("%s[%d]", name, index)]
		if !ok || len(found) == 0 {
			break
		}
		indexed = append(indexed, found[0])
	}
	if len(indexed) > 0 {
		return indexed
	}
	if found, ok := files[name+"[]"]; ok {
		return found
	}
	return nil
}

func isFileField(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	return t == reflect.TypeOf(multipart.FileHeader{})
}

func setField(field reflect.Value, raw []string) error {
	if field.Kind() == reflect.Slice {
		slice := reflect.MakeSlice(field.Type(), len(raw), len(raw))
		for i, value := range raw {
			if err := setScalar(slice.Index(i), value); err != nil {
				return err
			}
		}
		field.Set(slice)
		return nil
	}
	if len(raw) == 0 {
		return nil
	}
	return setScalar(field, raw[0])
}

func setScalar(field reflect.Value, raw string) error {
	if field.Kind() == reflect.Ptr {
		// An empty value leaves an optional field nil rather than pointing at a
		// zero: "not supplied" and "supplied as empty" are different answers,
		// and a pointer field is how a struct says it cares about the
		// difference.
		if raw == "" {
			field.Set(reflect.Zero(field.Type()))
			return nil
		}
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return setScalar(field.Elem(), raw)
	}

	// Before the Kind switch: a time.Time is a struct, and the generic struct
	// case cannot do anything useful with one.
	if field.Type() == reflect.TypeOf(time.Time{}) {
		if raw == "" {
			field.Set(reflect.ValueOf(time.Time{}))
			return nil
		}
		parsed, err := parseTime(raw)
		if err != nil {
			return err
		}
		field.Set(reflect.ValueOf(parsed))
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		// "on" is what an HTML checkbox sends, and the stdlib's ParseBool does
		// not know it. An unchecked box sends nothing at all, which is why a
		// missing key leaves the field false rather than erroring.
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "on", "yes":
			field.SetBool(true)
		case "", "off", "no":
			field.SetBool(false)
		default:
			parsed, err := strconv.ParseBool(raw)
			if err != nil {
				return fmt.Errorf("%q is not a true/false value", raw)
			}
			field.SetBool(parsed)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if raw == "" {
			field.SetInt(0)
			return nil
		}
		parsed, err := strconv.ParseInt(raw, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a whole number", raw)
		}
		field.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if raw == "" {
			field.SetUint(0)
			return nil
		}
		parsed, err := strconv.ParseUint(raw, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a positive whole number", raw)
		}
		field.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		if raw == "" {
			field.SetFloat(0)
			return nil
		}
		parsed, err := strconv.ParseFloat(raw, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a number", raw)
		}
		field.SetFloat(parsed)
	default:
		// Anything else is reported rather than skipped. It used to return
		// nil, so a field of a type this function did not know — a time.Time,
		// say — bound to its zero value with nothing said: the request
		// succeeded and the date was 0001-01-01. A binder that cannot bind a
		// field has to say so.
		return fmt.Errorf("cannot bind %q into a %s", raw, field.Type())
	}
	return nil
}

// timeFormats are what a form can carry a time in.
//
// RFC3339 first, because that is what an API client and our own JavaScript
// send. Then the two shapes HTML's own inputs produce, which carry no zone:
// <input type="datetime-local"> and <input type="date">. Those are read as
// UTC, because the alternative is the server's local zone — which makes the
// same form mean different instants on a laptop and in a container.
var timeFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02",
}

func parseTime(raw string) (time.Time, error) {
	for _, layout := range timeFormats {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("%q is not a date or time", raw)
}

// IsFormRequest reports whether the body is a form this package can decode.
func IsFormRequest(r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	return strings.HasPrefix(contentType, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(contentType, "multipart/form-data")
}
