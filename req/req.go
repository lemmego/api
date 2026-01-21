// Package req provides HTTP request parsing, validation, and input binding utilities.
//
// It supports automatic parsing of request data from various sources (JSON, form data,
// query parameters, headers) into Go structs with validation support. The package
// integrates with httpin for flexible input binding and provides validation interfaces
// for request data validation.
package req

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/ggicci/httpin"
	"github.com/ggicci/httpin/core"
	"github.com/golang/gddo/httputil/header"
)

const InKey = "input"

// Validator defines an interface for structs that can validate themselves.
// Types implementing this interface can provide custom validation logic.
type Validator interface {
	Validate() error
}

// RequestResponder provides access to HTTP request and response writer.
// This interface is typically implemented by framework context types.
type RequestResponder interface {
	Request() *http.Request
	ResponseWriter() http.ResponseWriter
}

// GetSetter provides key-value storage for request-scoped data.
// This allows storing and retrieving arbitrary values during request processing.
type GetSetter interface {
	Get(key string) any
	Set(key string, value any)
}

// Context combines request/response access with key-value storage.
// This interface is typically implemented by HTTP context types.
type Context interface {
	RequestResponder
	GetSetter
}

type MalformedRequest struct {
	Status  int
	Message string
}

func (mr *MalformedRequest) Error() string {
	return mr.Message
}

func WantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.HasSuffix(accept, "json")
}

func WantsHTML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html")
}

func WantsXML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/xml") || strings.Contains(accept, "text/xml")
}

func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	if r.Header.Get("Content-Type") != "" {
		value, _ := header.ParseValueAndParams(r.Header, "Content-Type")
		if value != "application/json" {
			msg := "Content-Type header is not application/json"
			return &MalformedRequest{Status: http.StatusUnsupportedMediaType, Message: msg}
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1048576)
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return &MalformedRequest{Status: http.StatusBadRequest, Message: err.Error()}
	}

	dec := json.NewDecoder(bytes.NewReader(bodyBytes))
	dec.DisallowUnknownFields()

	err = dec.Decode(&dst)

	// Repopulate the body for potential future-streaming from the buffer.
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError

		switch {
		case errors.As(err, &syntaxError):
			msg := fmt.Sprintf("Request body contains badly-formed JSON (at position %d)", syntaxError.Offset)
			return &MalformedRequest{Status: http.StatusBadRequest, Message: msg}

		case errors.Is(err, io.ErrUnexpectedEOF):
			msg := "Request body contains badly-formed JSON"
			return &MalformedRequest{Status: http.StatusBadRequest, Message: msg}

		case errors.As(err, &unmarshalTypeError):
			prefix := "json: cannot unmarshal string into Go value of type map[string]interface"
			if strings.HasPrefix(err.Error(), prefix) {
				msg := "Request body contains unprocessable JSON"
				return &MalformedRequest{Status: http.StatusBadRequest, Message: msg}
			}
			msg := fmt.Sprintf("Request body contains an invalid value for the %#q field (at position %d)", unmarshalTypeError.Field, unmarshalTypeError.Offset)
			return &MalformedRequest{Status: http.StatusBadRequest, Message: msg}

		case strings.HasPrefix(err.Error(), "json: unknown field "):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			msg := fmt.Sprintf("Request body contains unknown field %s", fieldName)
			return &MalformedRequest{Status: http.StatusBadRequest, Message: msg}

		case errors.Is(err, io.EOF):
			msg := "Request body must not be empty"
			return &MalformedRequest{Status: http.StatusBadRequest, Message: msg}

		case err.Error() == "http: request body too large":
			msg := "Request body must not be larger than 1MB"
			return &MalformedRequest{Status: http.StatusRequestEntityTooLarge, Message: msg}

		default:
			return err
		}
	}

	err = dec.Decode(&struct{}{})
	if err != io.EOF {
		msg := "Request body must only contain a single JSON object"
		return &MalformedRequest{Status: http.StatusBadRequest, Message: msg}
	}

	return nil
}

func HasMultiPart(r *http.Request) bool {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	return contentType != "" && strings.HasPrefix(contentType, "multipart/")
}

func HasFormData(r *http.Request) bool {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	return contentType != "" && strings.HasPrefix(contentType, "multipart/form-data")
}

func HasFormUrlEncoded(r *http.Request) bool {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	return contentType != "" && strings.HasPrefix(contentType, "application/x-www-form-urlencoded")
}

func HasJSON(r *http.Request) bool {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	return contentType != "" && strings.HasPrefix(contentType, "application/json")
}

func ParseInput(rr RequestResponder, inputStruct any, opts ...core.Option) error {
	if HasJSON(rr.Request()) {
		if err := DecodeJSONBody(rr.ResponseWriter(), rr.Request(), inputStruct); err != nil {
			return err
		}
		return nil
	}
	co, err := httpin.New(inputStruct, opts...)

	if err != nil {
		return err
	}

	input, err := co.Decode(rr.Request())
	if err != nil {
		return err
	}

	reflect.ValueOf(inputStruct).Elem().Set(reflect.ValueOf(input).Elem())

	return nil
}

func In(c Context, inputStruct any, opts ...core.Option) error {
	if HasJSON(c.Request()) {
		if err := DecodeJSONBody(c.ResponseWriter(), c.Request(), inputStruct); err != nil {
			return err
		}
		c.Set(InKey, inputStruct)
		return nil
	}
	co, err := httpin.New(inputStruct, opts...)

	if err != nil {
		return err
	}

	input, err := co.Decode(c.Request())
	if err != nil {
		return err
	}

	c.Set(InKey, input)
	return nil
}
