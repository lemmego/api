package req

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Media types this package knows how to turn into a struct.
const (
	MediaJSON          = "application/json"
	MediaXML           = "application/xml"
	MediaTextXML       = "text/xml"
	MediaForm          = "application/x-www-form-urlencoded"
	MediaMultipartForm = "multipart/form-data"
)

// BodyKind is the shape of a request body.
type BodyKind int

const (
	// BodyForm covers URL-encoded, multipart, and a request with no body at
	// all — a GET, where the query string is the form.
	BodyForm BodyKind = iota
	BodyJSON
	BodyXML
)

// KindOf reports how a request's body should be read.
//
// From the Content-Type and nothing else. Letting the target's struct tags
// influence it would parse the same request two different ways depending on
// what it was being parsed into, and two parsers disagree in ways a struct
// does not show.
//
// A suffixed type counts: `application/vnd.api+json` is JSON, and
// `application/atom+xml` is XML. An unknown type is a form, which is what the
// standard library's ParseForm assumes and the only guess that can be made
// from a body with no type at all.
func KindOf(r *http.Request) BodyKind {
	media := mediaType(r)
	switch {
	case media == MediaJSON || strings.HasSuffix(media, "+json"):
		return BodyJSON
	case media == MediaXML || media == MediaTextXML || strings.HasSuffix(media, "+xml"):
		return BodyXML
	default:
		return BodyForm
	}
}

// mediaType is the Content-Type without its parameters, lowercased.
func mediaType(r *http.Request) string {
	value := r.Header.Get("Content-Type")
	if index := strings.IndexByte(value, ';'); index >= 0 {
		value = value[:index]
	}
	return strings.ToLower(strings.TrimSpace(value))
}

// DecodeXMLBody reads an XML body onto a struct's `xml:` tags.
//
// Bounded at the same 1 MiB as the JSON decoder, and for a sharper reason:
// an XML parser is a traditional place to find a decompression bomb, and
// encoding/xml does not expand entities — but it will still happily read a
// gigabyte of nested elements if something hands it one.
func DecodeXMLBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if err.Error() == "http: request body too large" {
			return &MalformedRequest{
				Status:  http.StatusRequestEntityTooLarge,
				Message: "Request body must not be larger than 1MB",
			}
		}
		return &MalformedRequest{Status: http.StatusBadRequest, Message: err.Error()}
	}

	// Put it back, so a handler that wants the raw body can still read it —
	// the same courtesy DecodeJSONBody extends.
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	if len(bytes.TrimSpace(body)) == 0 {
		return &MalformedRequest{
			Status:  http.StatusBadRequest,
			Message: "Request body must not be empty",
		}
	}

	if err := xml.Unmarshal(body, dst); err != nil {
		var syntaxError *xml.SyntaxError
		if ok := asXMLSyntaxError(err, &syntaxError); ok {
			return &MalformedRequest{
				Status:  http.StatusBadRequest,
				Message: fmt.Sprintf("Request body contains badly-formed XML (at line %d)", syntaxError.Line),
			}
		}
		return &MalformedRequest{
			Status:  http.StatusBadRequest,
			Message: "Request body contains unprocessable XML",
		}
	}
	return nil
}

func asXMLSyntaxError(err error, target **xml.SyntaxError) bool {
	if syntaxError, ok := err.(*xml.SyntaxError); ok {
		*target = syntaxError
		return true
	}
	return false
}
