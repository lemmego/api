package app

import (
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/lemmego/api/shared"
)

type Validator struct {
	AppManager
	Errors shared.ValidationErrors
}

func NewValidator(app AppManager) *Validator {
	return &Validator{
		AppManager: app,
		Errors:     make(map[string][]string),
	}
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
func (v *Validator) Field(name string, value interface{}) *VField {
	return &VField{
		vee:   v,
		name:  name,
		value: value,
	}
}

type VField struct {
	vee   *Validator
	name  string
	value interface{}
}

func (f *VField) Value() interface{} {
	return f.value
}

func (f *VField) SetValue(value interface{}) *VField {
	f.value = value
	return f
}

func (f *VField) Name() string {
	return f.name
}

// Required checks if the value is not empty
func (f *VField) Required() *VField {
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
		f.vee.AddError(f.name, "This field is required")
	}
	return f
}

// Equals checks if the value is equal to the provided value
func (f *VField) Equals(value interface{}) *VField {
	if f.value != value {
		f.vee.AddError(f.name, "This field must match with the provided value")
	}
	return f
}

// Min checks if the value is greater than or equal to the minimum
func (f *VField) Min(min int) *VField {
	if v, ok := f.value.(int); ok {
		if v < min {
			f.vee.AddError(f.name, "This field must be at least "+strconv.Itoa(min))
		}
	}
	return f
}

// Max checks if the value is less than or equal to the maximum
func (f *VField) Max(max int) *VField {
	if v, ok := f.value.(int); ok {
		if v > max {
			f.vee.AddError(f.name, "This field must not exceed "+strconv.Itoa(max))
		}
	}
	return f
}

// Between checks if the value is between min and max (inclusive)
func (f *VField) Between(min, max int) *VField {
	if v, ok := f.value.(int); ok {
		if v < min || v > max {
			f.vee.AddError(f.name, fmt.Sprintf("This field must be between %d and %d", min, max))
		}
	}
	return f
}

// Email checks if the value is a valid email address
func (f *VField) Email() *VField {
	if v, ok := f.value.(string); ok {
		emailRegex := regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
		if !emailRegex.MatchString(v) {
			f.vee.AddError(f.name, "This field must be a valid email address")
		}
	}
	return f
}

// Alpha checks if the value contains only alphabetic characters
func (f *VField) Alpha() *VField {
	if v, ok := f.value.(string); ok {
		for _, char := range v {
			if !unicode.IsLetter(char) {
				f.vee.AddError(f.name, "This field must contain only alphabetic characters")
				break
			}
		}
	}
	return f
}

// Numeric checks if the value contains only numeric characters
func (f *VField) Numeric() *VField {
	if v, ok := f.value.(string); ok {
		for _, char := range v {
			if !unicode.IsDigit(char) {
				f.vee.AddError(f.name, "This field must contain only numeric characters")
				break
			}
		}
	}
	return f
}

// AlphaNumeric checks if the value contains only alphanumeric characters
func (f *VField) AlphaNumeric() *VField {
	if v, ok := f.value.(string); ok {
		for _, char := range v {
			if !unicode.IsLetter(char) && !unicode.IsDigit(char) {
				f.vee.AddError(f.name, "This field must contain only alphanumeric characters")
				break
			}
		}
	}
	return f
}

// Date checks if the value is a valid date in the specified format
func (f *VField) Date(layout string) *VField {
	if v, ok := f.value.(string); ok {
		_, err := time.Parse(layout, v)
		if err != nil {
			f.vee.AddError(f.name, "This field must be a valid date in the format "+layout)
		}
	}
	return f
}

// In checks if the value is in the given slice of valid values
func (f *VField) In(validValues []string) *VField {
	if v, ok := f.value.(string); ok {
		for _, validValue := range validValues {
			if v == validValue {
				return f
			}
		}
		f.vee.AddError(f.name, "This field must be one of the following: "+strings.Join(validValues, ", "))
	}
	return f
}

// Regex checks if the value matches the given regular expression
func (f *VField) Regex(pattern string) *VField {
	if v, ok := f.value.(string); ok {
		regex, err := regexp.Compile(pattern)
		if err != nil {
			f.vee.AddError(f.name, "Invalid regular expression pattern")
		} else if !regex.MatchString(v) {
			f.vee.AddError(f.name, "This field must match the pattern: "+pattern)
		}
	}
	return f
}

// URL checks if the value is a valid URL
func (f *VField) URL() *VField {
	if v, ok := f.value.(string); ok {
		_, err := url.ParseRequestURI(v)
		if err != nil {
			f.vee.AddError(f.name, "This field must be a valid URL")
		}
	}
	return f
}

// IP checks if the value is a valid IP address (v4 or v6)
func (f *VField) IP() *VField {
	if v, ok := f.value.(string); ok {
		ip := net.ParseIP(v)
		if ip == nil {
			f.vee.AddError(f.name, "This field must be a valid IP address")
		}
	}
	return f
}

// UUID checks if the value is a valid UUID
func (f *VField) UUID() *VField {
	if v, ok := f.value.(string); ok {
		_, err := uuid.Parse(v)
		if err != nil {
			f.vee.AddError(f.name, "This field must be a valid UUID")
		}
	}
	return f
}

// Boolean checks if the value is a valid boolean
func (f *VField) Boolean() *VField {
	switch f.value.(type) {
	case bool:
		return f
	case string:
		lowercaseValue := strings.ToLower(f.value.(string))
		if lowercaseValue != "true" && lowercaseValue != "false" {
			f.vee.AddError(f.name, "This field must be a boolean value")
		}
	case int:
		intValue := f.value.(int)
		if intValue != 0 && intValue != 1 {
			f.vee.AddError(f.name, "This field must be a boolean value")
		}
	default:
		f.vee.AddError(f.name, "This field must be a boolean value")
	}
	return f
}

// JSON checks if the value is a valid JSON string
func (f *VField) JSON() *VField {
	if v, ok := f.value.(string); ok {
		var js json.RawMessage
		if json.Unmarshal([]byte(v), &js) != nil {
			f.vee.AddError(f.name, "This field must be a valid JSON string")
		}
	}
	return f
}

// AfterDate checks if the date is after the specified date
func (f *VField) AfterDate(afterDate time.Time) *VField {
	if v, ok := f.value.(time.Time); ok {
		if !v.After(afterDate) {
			f.vee.AddError(f.name, "This field must be a date after "+afterDate.String())
		}
	}
	return f
}

// BeforeDate checks if the date is before the specified date
func (f *VField) BeforeDate(beforeDate time.Time) *VField {
	if v, ok := f.value.(time.Time); ok {
		if !v.Before(beforeDate) {
			f.vee.AddError(f.name, "This field must be a date before "+beforeDate.String())
		}
	}
	return f
}

// StartsWith checks if the string starts with the specified substring
func (f *VField) StartsWith(prefix string) *VField {
	if v, ok := f.value.(string); ok {
		if !strings.HasPrefix(v, prefix) {
			f.vee.AddError(f.name, "This field must start with "+prefix)
		}
	}
	return f
}

// EndsWith checks if the string ends with the specified substring
func (f *VField) EndsWith(suffix string) *VField {
	if v, ok := f.value.(string); ok {
		if !strings.HasSuffix(v, suffix) {
			f.vee.AddError(f.name, "This field must end with "+suffix)
		}
	}
	return f
}

// Contains checks if the string contains the specified substring
func (f *VField) Contains(substring string) *VField {
	if v, ok := f.value.(string); ok {
		if !strings.Contains(v, substring) {
			f.vee.AddError(f.name, "This field must contain "+substring)
		}
	}
	return f
}

// Dimensions checks if the image file has the specified dimensions
func (f *VField) Dimensions(width, height int) *VField {
	if v, ok := f.value.(string); ok {
		file, err := os.Open(v)
		if err != nil {
			f.vee.AddError(f.name, "Unable to open the file")
			return f
		}
		defer file.Close()

		img, _, err := image.DecodeConfig(file)
		if err != nil {
			f.vee.AddError(f.name, "Unable to decode the image")
			return f
		}

		if img.Width != width || img.Height != height {
			f.vee.AddError(f.name, fmt.Sprintf("Image dimensions must be %dx%d", width, height))
		}
	}
	return f
}

// MimeTypes checks if the file has one of the specified MIME types
func (f *VField) MimeTypes(allowedTypes []string) *VField {
	if v, ok := f.value.(string); ok {
		file, err := os.Open(v)
		if err != nil {
			f.vee.AddError(f.name, "Unable to open the file")
			return f
		}
		defer file.Close()

		buffer := make([]byte, 512)
		_, err = file.Read(buffer)
		if err != nil && err != io.EOF {
			f.vee.AddError(f.name, "Unable to read the file")
			return f
		}

		mimeType := http.DetectContentType(buffer)

		for _, allowedType := range allowedTypes {
			if mimeType == allowedType {
				return f
			}
		}

		f.vee.AddError(f.name, "File type must be one of: "+strings.Join(allowedTypes, ", "))
	}
	return f
}

// Timezone checks if the value is a valid timezone
func (f *VField) Timezone() *VField {
	if v, ok := f.value.(string); ok {
		_, err := time.LoadLocation(v)
		if err != nil {
			f.vee.AddError(f.name, "Invalid timezone")
		}
	}
	return f
}

// ActiveURL checks if the URL is active and reachable
func (f *VField) ActiveURL() *VField {
	if v, ok := f.value.(string); ok {
		resp, err := http.Get(v)
		if err != nil {
			f.vee.AddError(f.name, "The URL is not active or reachable")
			return f
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			f.vee.AddError(f.name, "The URL returned a non-OK status")
		}
	}
	return f
}

// AlphaDash checks if the string contains only alpha-numeric characters, dashes, or underscores
func (f *VField) AlphaDash() *VField {
	if v, ok := f.value.(string); ok {
		re := regexp.MustCompile("^[a-zA-Z0-9-_]+$")
		if !re.MatchString(v) {
			f.vee.AddError(f.name, "This field may only contain alpha-numeric characters, dashes, and underscores")
		}
	}
	return f
}

// Ascii checks if the string contains only ASCII characters
func (f *VField) Ascii() *VField {
	if v, ok := f.value.(string); ok {
		for _, char := range v {
			if char > unicode.MaxASCII {
				f.vee.AddError(f.name, "This field may only contain ASCII characters")
				break
			}
		}
	}
	return f
}

// MacAddress checks if the string is a valid MAC address
func (f *VField) MacAddress() *VField {
	if v, ok := f.value.(string); ok {
		_, err := net.ParseMAC(v)
		if err != nil {
			f.vee.AddError(f.name, "This field must be a valid MAC address")
		}
	}
	return f
}

// ULID checks if the string is a valid ULID
func (f *VField) ULID() *VField {
	if v, ok := f.value.(string); ok {
		re := regexp.MustCompile("^[0-9A-HJKMNP-TV-Z]{26}$")
		if !re.MatchString(v) {
			f.vee.AddError(f.name, "This field must be a valid ULID")
		}
	}
	return f
}

// Distinct checks if all elements in a slice are unique
func (f *VField) Distinct() *VField {
	if slice, ok := f.value.([]interface{}); ok {
		seen := make(map[interface{}]bool)
		for _, value := range slice {
			if seen[value] {
				f.vee.AddError(f.name, "This field must contain only unique values")
				break
			}
			seen[value] = true
		}
	}
	return f
}

// Filled checks if the value is not empty (for strings, slices, maps, and pointers)
func (f *VField) Filled() *VField {
	switch val := f.value.(type) {
	case string:
		if val == "" {
			f.vee.AddError(f.name, "This field must be filled")
		}
	case []interface{}:
		if len(val) == 0 {
			f.vee.AddError(f.name, "This field must be filled")
		}
	case map[string]interface{}:
		if len(val) == 0 {
			f.vee.AddError(f.name, "This field must be filled")
		}
	case nil:
		f.vee.AddError(f.name, "This field must be filled")
	}
	return f
}

// HexColor checks if the string is a valid hexadecimal color code
func (f *VField) HexColor() *VField {
	if v, ok := f.value.(string); ok {
		re := regexp.MustCompile("^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$")
		if !re.MatchString(v) {
			f.vee.AddError(f.name, "This field must be a valid hexadecimal color code")
		}
	}
	return f
}

func (f *VField) Unique(table string, column string) *VField {
	var count int64
	f.vee.DB().Table(table).Where(fmt.Sprintf("%s = ?", column), f.value).Count(&count)

	if count > 0 {
		f.vee.AddError(f.name, "This field must be unique")
	}

	return f
}

// ForEach applies validation rules to each item in an array
func (f *VField) ForEach(rules ...func(*VField) *VField) *VField {
	slice := reflect.ValueOf(f.value)

	if slice.Kind() == reflect.Ptr {
		slice = slice.Elem()
	}

	if slice.Kind() != reflect.Slice && slice.Kind() != reflect.Array {
		f.vee.AddError(f.name, "This field must be an array or slice")
		return f
	}

	if slice.Len() == 0 {
		f.vee.AddError(f.name, "This field cannot be empty")
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
func (f *VField) Custom(validateFunc func(v interface{}) (bool, string)) *VField {
	if isValid, errorMessage := validateFunc(f.value); !isValid {
		f.vee.AddError(f.name, errorMessage)
	}
	return f
}
