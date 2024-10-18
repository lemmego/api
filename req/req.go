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

	"github.com/romsar/gonertia"

	"github.com/ggicci/httpin"
	"github.com/ggicci/httpin/core"
	"github.com/golang/gddo/httputil/header"
)

const InKey = "input"

type Validator interface {
	Validate() error
}

type RequestResponder interface {
	Request() *http.Request
	ResponseWriter() http.ResponseWriter
}

type GetSetter interface {
	Get(key string) interface{}
	Set(key string, value interface{})
}

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

func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst interface{}) error {
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

func HasFormData(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data")
}

func ParseInput(rr RequestResponder, inputStruct any, opts ...core.Option) error {
	if !HasFormData(rr.Request()) && (WantsJSON(rr.Request()) || gonertia.IsInertiaRequest(rr.Request())) {
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
	if WantsJSON(c.Request()) || gonertia.IsInertiaRequest(c.Request()) {
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
