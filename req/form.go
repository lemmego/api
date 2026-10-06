package req

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

// maxMultipartMemory is what is held in memory before a part spills to disk.
const maxMultipartMemory = 32 << 20 // 32 MiB

// HasFormTags reports whether a struct declares any `form:` tags.
//
// It is the signal that decides which decoder runs: a struct tagged for httpin
// (`in:"form=name"`) keeps using httpin, and one tagged `form:"name"` — which is
// what lemmego's own generators emit — is decoded here. Without this check,
// adding a second decoder would change the behaviour of every existing input.
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
		if tag := t.Field(i).Tag.Get("form"); tag != "" && tag != "-" {
			return true
		}
	}
	return false
}

// DecodeForm binds a URL-encoded or multipart form onto a struct by its `form:`
// tags.
//
// This exists because httpin — which handles every other binding path — reads
// only its own `in:` tag. A struct tagged `form:"name"`, which is what
// `lemmego g input` generates, bound nothing at all: a plain HTML form arrived
// empty and validation reported "this field is required" for every field the
// person had filled in. JSON requests were unaffected, which is why it survived
// so long: an Inertia app posts JSON.
//
// Arrays are accepted in both the shapes a browser and an HTTP client produce —
// repeated keys (`topic_ids=1&topic_ids=2`) and indexed keys
// (`topic_ids[0]=1&topic_ids[1]=2`).
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
		name := strings.Split(field.Tag.Get("form"), ",")[0]
		if name == "" || name == "-" || !target.Field(i).CanSet() {
			continue
		}
		// A file is not a form value. It is read through FormFile, and
		// attempting to parse one as text would fail on every upload.
		if isFileField(field.Type) {
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
		// Anything else is left alone rather than guessed at. A struct field
		// that needs custom parsing is better served by reading the form in the
		// handler than by this function inventing a convention.
		return nil
	}
	return nil
}

// IsFormRequest reports whether the body is a form this package can decode.
func IsFormRequest(r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	return strings.HasPrefix(contentType, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(contentType, "multipart/form-data")
}
