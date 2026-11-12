package app

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/lemmego/api/shared"

	// image.DecodeConfig dispatches on registered formats, and registration
	// happens in a decoder's init. Without these, every call returns
	// image.ErrFormat and an Image rule rejects every image — which is worse
	// than having no rule, because the form looks validated.
	//
	// These three cover what a browser's file picker produces. WebP and AVIF
	// are not in the standard library; a project that accepts them needs
	// golang.org/x/image and can register it itself, since the registry is
	// process-global.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

type Validator struct {
	Errors          shared.ValidationErrors
	activeURLConfig activeURLConfig
	// err holds the first failure from a rule that could not run, as opposed
	// to a value that was invalid. See Check and Err.
	err error

	// t translates a rule's message key. nil means the embedded English in
	// validation_messages.go, which is what a unit test and a project that has
	// not registered a translator both get.
	t Translator

	// attributes gives a field a human label for :attribute, set per
	// validator by Attributes.
	attributes map[string]string
}

// ValidatorOption configures a validator at construction.
type ValidatorOption func(*Validator)

// WithTranslator gives a validator the request's translator, so its messages
// come out in the reader's language.
//
// ctx.Validator() passes this, which is the construction path every real
// caller takes. A bare NewValidator() keeps working and reads in English.
func WithTranslator(t Translator) ValidatorOption {
	return func(v *Validator) { v.t = t }
}

func NewValidator(opts ...ValidatorOption) *Validator {
	v := &Validator{
		Errors:          make(map[string][]string),
		activeURLConfig: defaultActiveURLConfig(),
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// Attributes gives this validator's fields human labels, which are what
// :attribute renders as.
//
//	v.Attributes(map[string]string{"email": "Email address"})
//
// Chainable so it reads as part of a one-expression Validate.
func (v *Validator) Attributes(names map[string]string) *Validator {
	if v.attributes == nil {
		v.attributes = make(map[string]string, len(names))
	}
	for field, label := range names {
		v.attributes[field] = label
	}
	return v
}

// translate resolves a message key for a field.
//
// Rendered now rather than at response time, and the reason is the Inertia
// path: ValidationError puts the rendered map in the session and redirects,
// so a deferred {key, params} would have to survive gob with parameters of
// arbitrary type — and an application's own Custom rule passing its own
// struct would panic at session-write time, inside a redirect, on a
// validation failure. Rendering here keeps shared.ValidationErrors exactly
// what it was, which is what gives its four consumers a zero-line diff.
//
// The one cost is that the locale is the failing request's, not that of the
// GET which renders the errors after Back(). That is why the negotiating
// middleware persists a locale taken from a query parameter to the cookie;
// the cookie and header sources are unaffected.
func (v *Validator) translate(key, field string, params map[string]any) string {
	repl := make(map[string]any, len(params)+1)
	repl["attribute"] = v.attribute(field)
	for name, value := range params {
		repl[name] = value
	}

	if v.t != nil && v.t.Has(key) {
		return v.t.Get(key, repl)
	}
	if pattern, ok := builtinMessages[key]; ok {
		return interpolate(pattern, repl)
	}
	// Not a key the framework knows, so it is a message — which is what keeps
	// Custom's and Check's existing signatures working.
	return interpolate(key, repl)
}

// attribute returns a field's human label.
//
// Never empty, so no message can render with a hole in it where a field name
// should be.
func (v *Validator) attribute(field string) string {
	if label, ok := v.attributes[field]; ok {
		return label
	}
	if v.t != nil {
		key := "validation.attributes." + field
		if v.t.Has(key) {
			return v.t.Get(key)
		}
		// A field inside ForEach is named "tags.0", so a catalogue cannot
		// list every index. One more lookup with the indices collapsed.
		if generic := collapseIndices(field); generic != field {
			if key := "validation.attributes." + generic; v.t.Has(key) {
				return v.t.Get(key)
			}
		}
	}
	return humaniseField(field)
}

// collapseIndices turns "tags.0.name" into "tags.*.name".
func collapseIndices(field string) string {
	parts := strings.Split(field, ".")
	changed := false
	for i, part := range parts {
		if part == "" {
			continue
		}
		if _, err := strconv.Atoi(part); err == nil {
			parts[i] = "*"
			changed = true
		}
	}
	if !changed {
		return field
	}
	return strings.Join(parts, ".")
}

// humaniseField is the last resort for :attribute: the field's own name, made
// readable. It matches what api/db's rules already do, so the framework has
// one convention rather than two.
func humaniseField(field string) string {
	readable := strings.ReplaceAll(field, "_", " ")
	return strings.ReplaceAll(readable, ".", " ")
}

// fail records a failure against a field under a message key.
//
// rule is appended to "validation.", so a rule passes "required" or
// "min.file".
func (f *vField) fail(rule string, params map[string]any) {
	f.vee.AddError(f.name, f.vee.translate("validation."+rule, f.name, params))
}

// ActiveURLConfig controls the network access performed by ActiveURL.
// Private targets are rejected unless AllowPrivateHosts is explicitly enabled.
type ActiveURLConfig struct {
	Client            *http.Client
	MaxResponseBytes  int64
	MaxRedirects      int
	Timeout           time.Duration
	AllowPrivateHosts bool
}

type activeURLConfig struct {
	ActiveURLConfig
}

func defaultActiveURLConfig() activeURLConfig {
	return activeURLConfig{ActiveURLConfig: ActiveURLConfig{
		Client:           &http.Client{Timeout: 10 * time.Second, Transport: safeActiveURLTransport()},
		MaxResponseBytes: 1 << 20,
		MaxRedirects:     5,
		Timeout:          10 * time.Second,
	}}
}

// NewValidatorWithActiveURLConfig creates a validator with explicit ActiveURL limits.
func NewValidatorWithActiveURLConfig(config ActiveURLConfig) *Validator {
	v := NewValidator()
	if config.Client != nil {
		v.activeURLConfig.Client = config.Client
	}
	if config.MaxResponseBytes > 0 {
		v.activeURLConfig.MaxResponseBytes = config.MaxResponseBytes
	}
	if config.MaxRedirects > 0 {
		v.activeURLConfig.MaxRedirects = config.MaxRedirects
	}
	if config.Timeout > 0 {
		v.activeURLConfig.Timeout = config.Timeout
		if config.Client == nil {
			v.activeURLConfig.Client = &http.Client{Timeout: config.Timeout, Transport: safeActiveURLTransport()}
		}
	}
	v.activeURLConfig.AllowPrivateHosts = config.AllowPrivateHosts
	return v
}

func (v *Validator) AddError(field, message string) {
	v.Errors[field] = append(v.Errors[field], message)
}

func (v *Validator) IsValid() bool {
	return len(v.Errors) == 0
}

func (v *Validator) Validate() error {
	if v.IsValid() {
		return nil
	}
	return v.Errors
}

func (v *Validator) ErrorsJSON() map[string][]string {
	return v.Errors
}

// Field creates a new Field instance for chaining validation rules
func (v *Validator) Field(name string, value any) *vField {
	return &vField{
		vee:   v,
		name:  name,
		value: value,
	}
}

type vField struct {
	vee   *Validator
	name  string
	value any
}

func (f *vField) Value() any {
	return f.value
}

func (f *vField) SetValue(value any) *vField {
	f.value = value
	return f
}

func (f *vField) Name() string {
	return f.name
}

// Required checks if the value is not empty
func (f *vField) Required() *vField {
	isZero := false

	switch v := f.value.(type) {
	case nil:
		isZero = true
	case string:
		isZero = v == ""
	case int, int8, int16, int32, int64:
		isZero = reflect.ValueOf(v).Int() == 0
	case uint, uint8, uint16, uint32, uint64:
		isZero = reflect.ValueOf(v).Uint() == 0
	case float32, float64:
		isZero = reflect.ValueOf(v).Float() == 0
	case bool:
		isZero = !v
	default:
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
			isZero = rv.IsNil()
		} else {
			isZero = rv.IsZero()
		}
	}

	if isZero {
		f.fail("required", nil)
	}
	return f
}

// Equals checks if the value is equal to the provided value
func (f *vField) Equals(value any) *vField {
	if f.value != value {
		f.fail("equals", nil)
	}
	return f
}

// Min checks if the value is greater than or equal to the minimum
func (f *vField) Min(min int) *vField {
	if v, ok := f.value.(int); ok {
		if v < min {
			f.fail("min.numeric", map[string]any{"min": min})
		}
	}
	return f
}

// Max checks if the value is less than or equal to the maximum
func (f *vField) Max(max int) *vField {
	if v, ok := f.value.(int); ok {
		if v > max {
			f.fail("max.numeric", map[string]any{"max": max})
		}
	}
	return f
}

// Between checks if the value is between min and max (inclusive)
func (f *vField) Between(min, max int) *vField {
	if v, ok := f.value.(int); ok {
		if v < min || v > max {
			f.fail("between.numeric", map[string]any{"min": min, "max": max})
		}
	}
	return f
}

// Email checks if the value is a valid email address
func (f *vField) Email() *vField {
	if v, ok := f.value.(string); ok {
		emailRegex := regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
		if !emailRegex.MatchString(v) {
			f.fail("email", nil)
		}
	}
	return f
}

// Alpha checks if the value contains only alphabetic characters
func (f *vField) Alpha() *vField {
	if v, ok := f.value.(string); ok {
		for _, char := range v {
			if !unicode.IsLetter(char) {
				f.fail("alpha", nil)
				break
			}
		}
	}
	return f
}

// Numeric checks if the value contains only numeric characters
func (f *vField) Numeric() *vField {
	if v, ok := f.value.(string); ok {
		for _, char := range v {
			if !unicode.IsDigit(char) {
				f.fail("numeric", nil)
				break
			}
		}
	}
	return f
}

// AlphaNumeric checks if the value contains only alphanumeric characters
func (f *vField) AlphaNumeric() *vField {
	if v, ok := f.value.(string); ok {
		for _, char := range v {
			if !unicode.IsLetter(char) && !unicode.IsDigit(char) {
				f.fail("alpha_num", nil)
				break
			}
		}
	}
	return f
}

// Date checks if the value is a valid date in the specified format
func (f *vField) Date(layout string) *vField {
	if v, ok := f.value.(string); ok {
		_, err := time.Parse(layout, v)
		if err != nil {
			f.fail("date_format", map[string]any{"format": layout})
		}
	}
	return f
}

// In checks if the value is in the given slice of valid values
func (f *vField) In(validValues []string) *vField {
	if v, ok := f.value.(string); ok {
		for _, validValue := range validValues {
			if v == validValue {
				return f
			}
		}
		f.fail("in", map[string]any{"values": strings.Join(validValues, ", ")})
	}
	return f
}

// Regex checks if the value matches the given regular expression
func (f *vField) Regex(pattern string) *vField {
	if v, ok := f.value.(string); ok {
		regex, err := regexp.Compile(pattern)
		if err != nil {
			f.fail("regex_pattern_invalid", nil)
		} else if !regex.MatchString(v) {
			f.fail("regex", map[string]any{"pattern": pattern})
		}
	}
	return f
}

// URL checks if the value is a valid URL
func (f *vField) URL() *vField {
	if v, ok := f.value.(string); ok {
		_, err := url.ParseRequestURI(v)
		if err != nil {
			f.fail("url", nil)
		}
	}
	return f
}

// IP checks if the value is a valid IP address (v4 or v6)
func (f *vField) IP() *vField {
	if v, ok := f.value.(string); ok {
		ip := net.ParseIP(v)
		if ip == nil {
			f.fail("ip", nil)
		}
	}
	return f
}

// UUID checks if the value is a valid UUID
func (f *vField) UUID() *vField {
	if v, ok := f.value.(string); ok {
		_, err := uuid.Parse(v)
		if err != nil {
			f.fail("uuid", nil)
		}
	}
	return f
}

// Boolean checks if the value is a valid boolean
func (f *vField) Boolean() *vField {
	switch f.value.(type) {
	case bool:
		return f
	case string:
		lowercaseValue := strings.ToLower(f.value.(string))
		if lowercaseValue != "true" && lowercaseValue != "false" {
			f.fail("boolean", nil)
		}
	case int:
		intValue := f.value.(int)
		if intValue != 0 && intValue != 1 {
			f.fail("boolean", nil)
		}
	default:
		f.fail("boolean", nil)
	}
	return f
}

// JSON checks if the value is a valid JSON string
func (f *vField) JSON() *vField {
	if v, ok := f.value.(string); ok {
		var js json.RawMessage
		if json.Unmarshal([]byte(v), &js) != nil {
			f.fail("json", nil)
		}
	}
	return f
}

// AfterDate checks if the date is after the specified date
func (f *vField) AfterDate(afterDate time.Time) *vField {
	if v, ok := f.value.(time.Time); ok {
		if !v.After(afterDate) {
			f.fail("after", map[string]any{"date": afterDate.String()})
		}
	}
	return f
}

// BeforeDate checks if the date is before the specified date
func (f *vField) BeforeDate(beforeDate time.Time) *vField {
	if v, ok := f.value.(time.Time); ok {
		if !v.Before(beforeDate) {
			f.fail("before", map[string]any{"date": beforeDate.String()})
		}
	}
	return f
}

// StartsWith checks if the string starts with the specified substring
func (f *vField) StartsWith(prefix string) *vField {
	if v, ok := f.value.(string); ok {
		if !strings.HasPrefix(v, prefix) {
			f.fail("starts_with", map[string]any{"value": prefix})
		}
	}
	return f
}

// EndsWith checks if the string ends with the specified substring
func (f *vField) EndsWith(suffix string) *vField {
	if v, ok := f.value.(string); ok {
		if !strings.HasSuffix(v, suffix) {
			f.fail("ends_with", map[string]any{"value": suffix})
		}
	}
	return f
}

// Contains checks if the string contains the specified substring
func (f *vField) Contains(substring string) *vField {
	if v, ok := f.value.(string); ok {
		if !strings.Contains(v, substring) {
			f.fail("contains", map[string]any{"value": substring})
		}
	}
	return f
}

// Dimensions checks if the image file has the specified dimensions
func (f *vField) Dimensions(width, height int) *vField {
	if v, ok := f.value.(string); ok {
		file, err := os.Open(v)
		if err != nil {
			f.fail("file_unopenable", nil)
			return f
		}
		defer file.Close()

		img, _, err := image.DecodeConfig(file)
		if err != nil {
			f.fail("file_undecodable", nil)
			return f
		}

		if img.Width != width || img.Height != height {
			f.fail("dimensions", map[string]any{"width": width, "height": height})
		}
	}
	return f
}

// MimeTypes checks if the file has one of the specified MIME types
func (f *vField) MimeTypes(allowedTypes []string) *vField {
	if v, ok := f.value.(string); ok {
		file, err := os.Open(v)
		if err != nil {
			f.fail("file_unopenable", nil)
			return f
		}
		defer file.Close()

		buffer := make([]byte, 512)
		_, err = file.Read(buffer)
		if err != nil && err != io.EOF {
			f.fail("file_unreadable", nil)
			return f
		}

		mimeType := http.DetectContentType(buffer)

		for _, allowedType := range allowedTypes {
			if mimeType == allowedType {
				return f
			}
		}

		f.fail("file_type", map[string]any{"values": strings.Join(allowedTypes, ", ")})
	}
	return f
}

// Timezone checks if the value is a valid timezone
func (f *vField) Timezone() *vField {
	if v, ok := f.value.(string); ok {
		_, err := time.LoadLocation(v)
		if err != nil {
			f.fail("timezone", nil)
		}
	}
	return f
}

// ActiveURL checks if the URL is active and reachable
func (f *vField) ActiveURL() *vField {
	if v, ok := f.value.(string); ok {
		config := f.vee.activeURLConfig
		target, err := url.ParseRequestURI(v)
		if err != nil || target.Scheme != "http" && target.Scheme != "https" || target.Hostname() == "" {
			f.fail("active_url", nil)
			return f
		}
		if !config.AllowPrivateHosts {
			if err := validateActiveURLHost(target, config.Timeout); err != nil {
				f.fail("active_url", nil)
				return f
			}
		}

		client := *config.Client
		client.Timeout = config.Timeout
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) > config.MaxRedirects {
				return fmt.Errorf("too many redirects")
			}
			if !config.AllowPrivateHosts {
				if err := validateActiveURLHost(req.URL, config.Timeout); err != nil {
					return err
				}
			}
			return nil
		}
		resp, err := client.Get(v)
		if err != nil {
			f.fail("active_url", nil)
			return f
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			f.fail("active_url_status", nil)
			return f
		}
		bytesRead, err := io.Copy(io.Discard, io.LimitReader(resp.Body, config.MaxResponseBytes+1))
		if err != nil {
			f.fail("active_url", nil)
		} else if bytesRead > config.MaxResponseBytes || resp.ContentLength > config.MaxResponseBytes {
			f.fail("active_url_too_large", nil)
		}
	}
	return f
}

func validateActiveURLHost(target *url.URL, timeout time.Duration) error {
	host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("private host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateActiveURLIP(ip) {
			return fmt.Errorf("private address")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("unable to resolve host")
	}
	for _, ip := range ips {
		if isPrivateActiveURLIP(ip) {
			return fmt.Errorf("private address")
		}
	}
	return nil
}

func isPrivateActiveURLIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func safeActiveURLTransport() http.RoundTripper {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if isPrivateActiveURLIP(ip) {
					return nil, fmt.Errorf("private address")
				}
			}
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}
}

// AlphaDash checks if the string contains only alpha-numeric characters, dashes, or underscores
func (f *vField) AlphaDash() *vField {
	if v, ok := f.value.(string); ok {
		re := regexp.MustCompile("^[a-zA-Z0-9-_]+$")
		if !re.MatchString(v) {
			f.fail("alpha_dash", nil)
		}
	}
	return f
}

// Ascii checks if the string contains only ASCII characters
func (f *vField) Ascii() *vField {
	if v, ok := f.value.(string); ok {
		for _, char := range v {
			if char > unicode.MaxASCII {
				f.fail("ascii", nil)
				break
			}
		}
	}
	return f
}

// MacAddress checks if the string is a valid MAC address
func (f *vField) MacAddress() *vField {
	if v, ok := f.value.(string); ok {
		_, err := net.ParseMAC(v)
		if err != nil {
			f.fail("mac_address", nil)
		}
	}
	return f
}

// ULID checks if the string is a valid ULID
func (f *vField) ULID() *vField {
	if v, ok := f.value.(string); ok {
		re := regexp.MustCompile("^[0-9A-HJKMNP-TV-Z]{26}$")
		if !re.MatchString(v) {
			f.fail("ulid", nil)
		}
	}
	return f
}

// Distinct checks if all elements in a slice are unique
func (f *vField) Distinct() *vField {
	if slice, ok := f.value.([]any); ok {
		seen := make(map[any]bool)
		for _, value := range slice {
			if seen[value] {
				f.fail("distinct", nil)
				break
			}
			seen[value] = true
		}
	}
	return f
}

// Filled checks if the value is not empty (for strings, slices, maps, and pointers)
func (f *vField) Filled() *vField {
	switch val := f.value.(type) {
	case string:
		if val == "" {
			f.fail("filled", nil)
		}
	case []interface{}:
		if len(val) == 0 {
			f.fail("filled", nil)
		}
	case map[string]interface{}:
		if len(val) == 0 {
			f.fail("filled", nil)
		}
	case nil:
		f.fail("filled", nil)
	}
	return f
}

// HexColor checks if the string is a valid hexadecimal color code
func (f *vField) HexColor() *vField {
	if v, ok := f.value.(string); ok {
		re := regexp.MustCompile("^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$")
		if !re.MatchString(v) {
			f.fail("hex_color", nil)
		}
	}
	return f
}

// ForEach applies validation rules to each item in an array
func (f *vField) ForEach(rules ...func(*vField) *vField) *vField {
	slice := reflect.ValueOf(f.value)

	if slice.Kind() == reflect.Ptr {
		slice = slice.Elem()
	}

	if slice.Kind() != reflect.Slice && slice.Kind() != reflect.Array {
		f.fail("array", nil)
		return f
	}

	if slice.Len() == 0 {
		f.fail("array_empty", nil)
		return f
	}

	for i := 0; i < slice.Len(); i++ {
		item := slice.Index(i).Interface()
		itemField := f.vee.Field(fmt.Sprintf("%s.%d", f.name, i), item)

		for _, rule := range rules {
			rule(itemField)
		}
	}

	return f
}

// Custom allows defining a custom validation rule
func (f *vField) Custom(validateFunc func(v interface{}) (bool, string)) *vField {
	if isValid, errorMessage := validateFunc(f.value); !isValid {
		f.vee.AddError(f.name, f.vee.translate(errorMessage, f.name, nil))
	}
	return f
}

// Extension

// Check runs a rule that can itself fail to run.
//
// Custom cannot express the difference between "this value is invalid" and "I
// could not tell". For a rule backed by a database that difference is the
// whole thing: if the lookup fails and the rule reports invalid, the person
// signing up is told their handle is taken when in fact the database is
// unreachable — and they will pick a different handle, permanently, over a
// transient outage.
//
// So fn returns (ok, message, err). An err records a message and is also kept
// on the Validator, where Err reports it, so a handler can answer 503 instead
// of 422.
func (f *vField) Check(fn func(value any) (bool, string, error)) *vField {
	ok, message, err := fn(f.value)
	if err != nil {
		f.vee.setErr(err)
		if message == "" {
			message = "validation.check_failed"
		}
		f.vee.AddError(f.name, f.vee.translate(message, f.name, nil))
		return f
	}
	if !ok {
		f.vee.AddError(f.name, f.vee.translate(message, f.name, nil))
	}
	return f
}

// setErr records the first rule failure that was not the value's fault.
func (v *Validator) setErr(err error) {
	if v.err == nil {
		v.err = err
	}
}

// Err returns the first error from a rule that could not run — a failed
// database lookup, say — or nil.
//
// It is separate from Validate because the two mean different things to the
// caller: Validate's error is the user's to fix, and this one is not. A
// handler that distinguishes them answers 422 for the first and 503 for the
// second.
func (v *Validator) Err() error { return v.err }

// ---------------------------------------------------------------------------
// Upload rules
//
// Dimensions and MimeTypes above accept only a filesystem path string: they
// begin with a type assertion to string and os.Open it. An uploaded file
// arrives as *multipart.FileHeader, so those rules silently do nothing — the
// assertion fails and the field is returned unchanged. A form that looked
// validated was not, which is worse than one that obviously is not.
//
// The rules below take the header. They read only what they need and always
// rewind, so a later rule and the handler still see the whole file.

// MaxSize fails when an uploaded file is larger than max bytes.
//
// Note that the multipart parser has already buffered the file by the time a
// rule sees it, so this bounds what gets stored, not what gets received. Use
// http.MaxBytesReader to bound the request itself.
func (f *vField) MaxSize(max int64) *vField {
	headers, ok := f.uploadHeaders()
	if !ok {
		return f
	}
	for _, header := range headers {
		if header.Size > max {
			f.failUpload(headers, header, "max.file", map[string]any{"max": humanBytes(max)})
		}
	}
	return f
}

// MinSize fails when an uploaded file is smaller than min bytes. A
// zero-length upload is usually a failed selection rather than a choice.
func (f *vField) MinSize(min int64) *vField {
	headers, ok := f.uploadHeaders()
	if !ok {
		return f
	}
	for _, header := range headers {
		if header.Size < min {
			f.failUpload(headers, header, "min.file", map[string]any{"min": humanBytes(min)})
		}
	}
	return f
}

// FileTypes fails unless an uploaded file's content is one of the given media
// types, for example "image/png".
//
// It sniffs the first 512 bytes rather than trusting the browser's
// Content-Type or the filename. Both are supplied by the client: a .png
// extension on an HTML file is how a stored upload becomes a cross-site
// scripting vector when it is later served from the same origin.
func (f *vField) FileTypes(types ...string) *vField {
	headers, ok := f.uploadHeaders()
	if !ok {
		return f
	}
	for _, header := range headers {
		detected, err := sniffUpload(header)
		if err != nil {
			f.failUpload(headers, header, "upload_unreadable", nil)
			continue
		}
		allowedType := slices.ContainsFunc(types, func(allowed string) bool {
			// DetectContentType appends parameters, as in "text/plain;
			// charset=utf-8", so compare the type and ignore them.
			return strings.EqualFold(mediaType(detected), mediaType(allowed))
		})
		if !allowedType {
			f.failUpload(headers, header, "mimes", map[string]any{"values": strings.Join(types, ", ")})
		}
	}
	return f
}

// Image fails unless the upload is a decodable image, and is the rule an
// avatar field wants: it proves the bytes are an image rather than something
// named like one.
func (f *vField) Image() *vField {
	headers, ok := f.uploadHeaders()
	if !ok {
		return f
	}
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			f.failUpload(headers, header, "upload_unreadable", nil)
			continue
		}
		_, _, err = image.DecodeConfig(file)
		_ = file.Close()
		if err != nil {
			f.failUpload(headers, header, "image", nil)
		}
	}
	return f
}

// ImageDimensions fails unless a decodable image's size is within bounds. A
// zero bound is ignored, so one side can be constrained alone.
func (f *vField) ImageDimensions(minWidth, minHeight, maxWidth, maxHeight int) *vField {
	headers, ok := f.uploadHeaders()
	if !ok {
		return f
	}
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			f.failUpload(headers, header, "upload_unreadable", nil)
			continue
		}
		config, _, err := image.DecodeConfig(file)
		_ = file.Close()
		if err != nil {
			f.failUpload(headers, header, "image", nil)
			continue
		}
		switch {
		case minWidth > 0 && config.Width < minWidth,
			minHeight > 0 && config.Height < minHeight:
			f.failUpload(headers, header, "image_dimensions.min", map[string]any{"width": minWidth, "height": minHeight})
		case maxWidth > 0 && config.Width > maxWidth,
			maxHeight > 0 && config.Height > maxHeight:
			f.failUpload(headers, header, "image_dimensions.max", map[string]any{"width": maxWidth, "height": maxHeight})
		}
	}
	return f
}

// uploadHeaders returns every upload in the field.
//
// A field that is not an upload is left alone rather than failed: the rules
// above are chained after Required, which is what decides whether an absent
// file is acceptable. Failing here as well would report two errors for one
// missing field.
//
// It answers with every file, not the first. Returning one would have each
// rule check a single attachment and wave the rest through: four files where
// only the first is a PNG would pass FileTypes, and the HTML one behind it
// would be stored and later served from the application's own origin.
func (f *vField) uploadHeaders() ([]*multipart.FileHeader, bool) {
	switch value := f.value.(type) {
	case *multipart.FileHeader:
		if value == nil {
			return nil, false
		}
		return []*multipart.FileHeader{value}, true
	case multipart.FileHeader:
		return []*multipart.FileHeader{&value}, true
	case []*multipart.FileHeader:
		headers := make([]*multipart.FileHeader, 0, len(value))
		for _, header := range value {
			if header != nil {
				headers = append(headers, header)
			}
		}
		return headers, len(headers) > 0
	}
	return nil, false
}

// failUpload is fail for one file within a possibly multi-file field.
//
// The separator and the ordering are a message key too, so a language that
// puts the filename elsewhere can say so. The filename is the client's own
// string, which is why the substituter must not rescan its output: a file
// named ":message.txt" has to stay literal rather than being re-read as a
// placeholder.
func (f *vField) failUpload(headers []*multipart.FileHeader, header *multipart.FileHeader, rule string, params map[string]any) {
	message := f.vee.translate("validation."+rule, f.name, params)
	if len(headers) > 1 && header.Filename != "" {
		message = f.vee.translate("validation.file_prefixed", f.name, map[string]any{
			"file":    header.Filename,
			"message": message,
		})
	}
	f.vee.AddError(f.name, message)
}

// sniffUpload reads the first 512 bytes and rewinds, so a later rule and the
// handler still see the whole file.
func sniffUpload(header *multipart.FileHeader) (string, error) {
	file, err := header.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && n == 0 {
		return "", err
	}
	return http.DetectContentType(buf[:n]), nil
}

func mediaType(contentType string) string {
	if idx := strings.IndexByte(contentType, ';'); idx >= 0 {
		return strings.TrimSpace(contentType[:idx])
	}
	return strings.TrimSpace(contentType)
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.0fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
