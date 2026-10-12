// Package req binds an HTTP request onto a Go struct.
//
// One decoder per shape of body and no more: encoding/json for a JSON body,
// and DecodeForm for everything else — URL-encoded, multipart, or a query
// string on a request that has no body. Which one runs is decided by the
// Content-Type, never by a struct tag.
//
// That last point is the whole design. There used to be two form binders,
// this one and httpin's, and a single tag on a single field decided which an
// input got. They disagreed about which array shapes bind, about whether a
// checkbox's `on` is true, and about whether files bind at all — and every
// one of those differences was discovered the same way, by something being
// dropped in silence while the request answered success.
package req

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
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
	return acceptsMediaType(r, "json")
}

func WantsHTML(r *http.Request) bool {
	return acceptsMediaType(r, "html")
}

func WantsXML(r *http.Request) bool {
	return acceptsMediaType(r, "xml")
}

func acceptsMediaType(r *http.Request, kind string) bool {
	bestSpecificity := -1
	accepted := false
	for _, headerValue := range r.Header.Values("Accept") {
		for _, value := range strings.Split(headerValue, ",") {
			mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(value))
			if err != nil {
				continue
			}

			quality := 1.0
			if rawQuality, ok := params["q"]; ok {
				quality, err = strconv.ParseFloat(rawQuality, 64)
				if err != nil || quality < 0 || quality > 1 {
					continue
				}
			}

			specificity, matches := mediaTypeSpecificity(mediaType, kind)
			if !matches || specificity < bestSpecificity {
				continue
			}
			if specificity > bestSpecificity || quality > 0 {
				bestSpecificity = specificity
				accepted = quality > 0
			}
		}
	}
	return accepted
}

func mediaTypeSpecificity(mediaType, kind string) (int, bool) {
	parts := strings.SplitN(mediaType, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, false
	}

	typePart, subtype := parts[0], parts[1]
	if typePart == "*" && subtype == "*" {
		return 0, true
	}
	if subtype == "*" {
		switch kind {
		case "json":
			return 1, typePart == "application"
		case "xml":
			return 1, typePart == "application" || typePart == "text"
		case "html":
			return 1, typePart == "text"
		}
		return 0, false
	}

	switch kind {
	case "json":
		return 2, typePart == "application" && (subtype == "json" || strings.HasSuffix(subtype, "+json"))
	case "html":
		return 2, typePart == "text" && subtype == "html"
	case "xml":
		return 2, (typePart == "application" || typePart == "text") && (subtype == "xml" || strings.HasSuffix(subtype, "+xml"))
	default:
		return 0, false
	}
}

func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	// Classified the same way ParseInput classifies, so the two cannot
	// disagree: a suffixed type such as application/vnd.api+json is JSON, and
	// a body with no type at all is allowed through because something has
	// evidently decided it is JSON by calling this.
	//
	// The previous check compared the media type to "application/json"
	// exactly, so every +json API was refused with 415 by a decoder that
	// would have parsed it perfectly.
	if media := mediaType(r); media != "" && KindOf(r) != BodyJSON {
		return &MalformedRequest{
			Status:  http.StatusUnsupportedMediaType,
			Message: fmt.Sprintf("Content-Type %s is not JSON", media),
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
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

// maxBodyBytes is how much of a body is read before it is refused.
//
// One megabyte: a JSON or XML document bigger than that is not a form
// submission, and reading an unbounded body into memory is how one request
// takes a process down. A file upload is not affected — it travels as
// multipart, which streams to disk past maxMultipartMemory.
const maxBodyBytes = 1 << 20

func HasFormUrlEncoded(r *http.Request) bool {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	return contentType != "" && strings.HasPrefix(contentType, "application/x-www-form-urlencoded")
}

func HasJSON(r *http.Request) bool {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	return contentType != "" && strings.HasPrefix(contentType, "application/json")
}

// ParseInput binds a request onto an input struct.
//
// Two shapes of body, and the Content-Type decides which:
//
//   - application/json, or anything ending +json, is decoded by encoding/json
//     onto `json:` tags.
//   - application/xml, text/xml, or anything ending +xml, is decoded by
//     encoding/xml onto `xml:` tags.
//   - anything else is a form — URL-encoded, multipart, or a query string on
//     a request with no body — and is decoded by DecodeForm, onto `form:` or
//     `in:"form=..."` tags.
//
// Which one runs is decided by the Content-Type and never by which tags the
// target happens to carry.
//
// This used to fall through to httpin for the second case unless the struct
// declared a `form:` tag, which made a single character on one field decide
// which of two binders an input got — and they disagreed about which array
// shapes bind, whether a checkbox's `on` is true, and whether files bind at
// all. Every one of those differences was found the same way: something was
// dropped in silence and a request answered 302.
func ParseInput(rr RequestResponder, inputStruct any) error {
	switch KindOf(rr.Request()) {
	case BodyJSON:
		return DecodeJSONBody(rr.ResponseWriter(), rr.Request(), inputStruct)
	case BodyXML:
		return DecodeXMLBody(rr.ResponseWriter(), rr.Request(), inputStruct)
	default:
		return DecodeForm(rr.Request(), inputStruct)
	}
}

// In is ParseInput, with the result also stashed on the context.
func In(c Context, inputStruct any) error {
	if err := ParseInput(c, inputStruct); err != nil {
		return err
	}
	c.Set(InKey, inputStruct)
	return nil
}
