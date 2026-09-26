// Package app provides HTTP request context and response utilities.
// The context system wraps HTTP requests and responses with framework-specific
// functionality including session management, validation, templating, and more.
package app

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/lemmego/api/session"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"unicode"

	"github.com/lemmego/api/shared"
	inertia "github.com/romsar/gonertia/v3"

	"github.com/lemmego/api/req"

	"github.com/a-h/templ"
)

func init() {
	gob.Register(shared.ValidationErrors{})
	gob.Register(map[string][]string{})
}

// Context represents an HTTP request context that provides access to request/response
// data, application services, session management, and response generation utilities.
// It serves as the primary interface for handling HTTP requests in route handlers.
type Context interface {
	GetSetter
	HttpProvider
	// App returns the application instance
	App() App
	// Next proceeds to the next middleware or handler in the chain
	Next() error
}

// GetSetter provides key-value storage for request-scoped data.
// This allows storing and retrieving arbitrary data during request processing.
type GetSetter interface {
	// Get retrieves a value by key from the context storage
	Get(key string) any
	// Set stores a value by key in the context storage
	Set(key string, value any)
}

// RequestGetSetter provides access to the underlying HTTP request.
type RequestGetSetter interface {
	// Request returns the underlying *http.Request
	Request() *http.Request
	// SetRequest sets the underlying *http.Request
	SetRequest(r *http.Request)
}

type HeaderGetSetter interface {
	Header(key string) string
	SetHeader(key string, value string) HeaderGetSetter
}

type RequestResponseResolver interface {
	Request() *http.Request
	ResponseWriter() http.ResponseWriter
	RequestContext() context.Context
}

type RequestBodyValidator interface {
	Validate(body req.Validator) error
	Validator() *Validator
}

type InputDecoder interface {
	ParseInput(inputStruct any) error
	Input(inputStruct any) any
	DecodeJSON(v interface{}) error
}

type BodyParser interface {
	Body() (map[string][]string, error)
	Form() (map[string][]string, error)
	FormFile(key string) (multipart.File, *multipart.FileHeader, error)
	HasFile(key string) bool
	HasMultiPartRequest() bool
	HasFormDataRequest() bool
	HasFormURLEncodedRequest() bool
	HasJSONRequest() bool
}

type AcceptHeaderResolver interface {
	WantsJSON() bool
	WantsHTML() bool
	WantsXML() bool
}

type ParamQueryResolver interface {
	Param(key string) string
	Query(key string) string
}

type CookieGetSetter interface {
	Cookie(name string) *http.Cookie
	SetCookie(cookie *http.Cookie) CookieGetSetter
}

type SessionGetSetter interface {
	Session(key string) any
	SessionString(key string) string
	PopSession(key string) any
	PopSessionString(key string) string
	PutSession(key string, value any) SessionGetSetter
}

type ErrorProvider interface {
	Error(status int, err error) error
	ValidationError(err error) error
	InternalServerError(err ...error) error
	NotFound(err ...error) error
	BadRequest(err ...error) error
	Unauthorized(err ...error) error
	Forbidden(err ...error) error
	PageExpired(err ...error) error
	MethodNotAllowed(err ...error) error
	UnprocessableEntity(err ...error) error
	NoContent() error
}

type FileResponder interface {
	StorageFile(path string, headers ...map[string][]string) error
	File(path string, headers ...map[string][]string) error
}

type HttpResponder interface {
	io.Writer
	FileResponder
	ResponseRenderer
	JSON(body M) error
	XML(v any) error
	Text(body []byte) error
	HTML(body []byte) error
	Redirect(url string) error
	Back() error
}

// Renderer defines the interface for types that can render content.
type Renderer interface {
	Render(w io.Writer) error
}

type ResponseRenderer interface {
	Render(r Renderer) error
}

type Downloader interface {
	Download(path string, filename string) error
}

type Uploader interface {
	Upload(uploadedFileName string, dir string, filename ...string) (*os.File, error)
}

type HttpProvider interface {
	ParamQueryResolver
	InputDecoder
	BodyParser
	RequestBodyValidator
	HeaderGetSetter
	AcceptHeaderResolver
	RequestGetSetter
	RequestResponseResolver
	CookieGetSetter
	SessionGetSetter
	HttpResponder
	Downloader
	Uploader
	ErrorProvider
	IsReading() bool
	Status() int
	SetStatus(code int) HttpResponder
	WriteStatus(code int) HttpResponder
	Referer() string
}

type ctx struct {
	sync.Mutex
	app     App
	request *http.Request
	writer  http.ResponseWriter
	status  int

	multipartFormOwned bool
	multipartFormOnce  sync.Once

	handlers []Handler
	index    int
}

func (c *ctx) Write(p []byte) (n int, err error) {
	return c.writer.Write(p)
}

func (c *ctx) WriteStatus(code int) HttpResponder {
	c.SetStatus(code)
	c.writer.WriteHeader(code)
	return c
}

func (c *ctx) Next() error {
	c.index++
	if c.index < len(c.handlers) {
		return c.handlers[c.index](c)
	}
	return nil
}

// SetCookie sets a cookie on the response writer
func (c *ctx) SetCookie(cookie *http.Cookie) CookieGetSetter {
	http.SetCookie(c.writer, cookie)
	return c
}

func (c *ctx) Cookie(name string) *http.Cookie {
	cookie, err := c.request.Cookie(name)
	if err != nil {
		return nil
	}

	return cookie
}

func (c *ctx) Validator() *Validator {
	return NewValidator()
}

func (c *ctx) Validate(body req.Validator) error {
	// return error if body is not a pointer
	if reflect.ValueOf(body).Kind() != reflect.Ptr {
		return errors.New("body must be a pointer")
	}

	if err := c.ParseInput(body); err != nil {
		return err
	}

	if err := body.Validate(); err != nil {
		return err
	}

	return nil
}

func (c *ctx) ParseInput(inputStruct any) error {
	err := req.ParseInput(c, inputStruct)
	if err != nil {
		return err
	}

	return nil
}

func (c *ctx) Input(inputStruct any) any {
	err := req.In(c, inputStruct)
	if err != nil {
		return nil
	}
	return c.Get(HTTPInKey)
}

func (c *ctx) SetInput(inputStruct any) error {
	err := req.In(c, inputStruct)
	if err != nil {
		return err
	}
	return nil
}

func (c *ctx) GetInput() any {
	return c.Get(HTTPInKey)
}

func (c *ctx) Render(r Renderer) error {
	if c.status != 0 {
		c.writer.WriteHeader(c.status)
	}
	return r.Render(c.ResponseWriter())
}

func (c *ctx) App() App {
	return c.app
}

func (c *ctx) Request() *http.Request {
	return c.request
}

func (c *ctx) ResponseWriter() http.ResponseWriter {
	return c.writer
}

func (c *ctx) RequestContext() context.Context {
	return c.request.Context()
}

func (c *ctx) Templ(component templ.Component) error {
	c.writer.Header().Set("content-type", "text/html")
	if c.status == 0 {
		c.status = http.StatusOK
	}
	c.writer.WriteHeader(c.status)
	return component.Render(c.Request().Context(), c.writer)
}

func (c *ctx) SetStatus(code int) HttpResponder {
	c.status = code
	return c
}

func (c *ctx) Status() int {
	return c.status
}

func (c *ctx) Header(key string) string {
	return c.request.Header.Get(key)
}

func (c *ctx) SetHeader(key string, value string) HeaderGetSetter {
	c.writer.Header().Add(key, value)
	return c
}

func (c *ctx) WantsJSON() bool {
	return req.WantsJSON(c.request)
}

func (c *ctx) WantsHTML() bool {
	return req.WantsHTML(c.request)
}

func (c *ctx) WantsXML() bool {
	return req.WantsXML(c.request)
}

func (c *ctx) JSON(body M) error {
	response, err := json.Marshal(body)
	if err != nil {
		return err
	}
	c.writer.Header().Set("content-Type", "application/json")
	if c.status == 0 {
		c.status = http.StatusOK
	}
	c.writer.WriteHeader(c.status)
	_, err = c.writer.Write(response)
	return err
}

func (c *ctx) XML(v any) error {
	var response []byte
	switch val := v.(type) {
	case []byte:
		response = val
	case string:
		response = []byte(val)
	case M, map[string]any:
		response = marshalMapToXML(val)
	default:
		var err error
		response, err = xml.Marshal(v)
		if err != nil {
			return err
		}
	}
	c.writer.Header().Set("content-Type", "application/xml")
	if c.status == 0 {
		c.status = http.StatusOK
	}
	c.writer.WriteHeader(c.status)
	_, err := c.writer.Write(response)
	return err
}

func marshalMapToXML(v any) []byte {
	var buf strings.Builder
	buf.WriteString("<response>")
	var mm map[string]any
	switch value := v.(type) {
	case M:
		mm = map[string]any(value)
	case map[string]any:
		mm = value
	}
	for k, val := range mm {
		name := xmlElementName(k)
		fmt.Fprintf(&buf, "<%s>%s</%s>", name, xmlEscape(fmt.Sprint(val)), name)
	}
	buf.WriteString("</response>")
	return []byte(buf.String())
}

func xmlElementName(s string) string {
	var buf strings.Builder
	for i, r := range s {
		valid := unicode.IsLetter(r) || r == '_'
		if i > 0 {
			valid = valid || unicode.IsDigit(r) || r == '-' || r == '.'
		}
		if valid {
			buf.WriteRune(r)
		} else {
			buf.WriteByte('_')
		}
	}
	if buf.Len() == 0 {
		return "_"
	}
	return buf.String()
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func (c *ctx) AuthUser(sessKey string) any {
	return c.PopSession(sessKey)
}

func (c *ctx) Text(body []byte) error {
	c.writer.Header().Set("content-type", "text/plain")
	if c.status == 0 {
		c.status = http.StatusOK
	}
	c.writer.WriteHeader(c.status)
	_, err := c.writer.Write(body)
	return err
}

func (c *ctx) HTML(body []byte) error {
	c.writer.Header().Set("content-type", "text/html")
	if c.status == 0 {
		c.status = http.StatusOK
	}
	c.writer.WriteHeader(c.status)
	_, err := c.writer.Write(body)
	return err
}

func (c *ctx) Redirect(url string) error {
	c.writer.Header().Set("Location", url)
	if c.status == 0 {
		c.status = http.StatusFound
	}
	c.WriteStatus(c.status)
	return nil
}

func (c *ctx) WithInput() *ctx {
	body, err := c.Form()
	if err == nil && body != nil {
		c.PutSession("input", body)
	}
	return c
}

func (c *ctx) Back() error {
	target := c.Referer()
	if target == "" {
		// Without a Referer there is nothing to go "back" to, and redirecting
		// to an empty location is not a response any client can follow. The
		// current path re-renders the page the request came from.
		target = c.Request().URL.Path
		if target == "" {
			target = "/"
		}
	}
	return c.Redirect(target)
}

func (c *ctx) Referer() string {
	return c.request.Referer()
}

func (c *ctx) HasMultiPartRequest() bool {
	return req.HasMultiPart(c.request)
}

func (c *ctx) HasFormDataRequest() bool {
	return req.HasFormData(c.request)
}

func (c *ctx) HasFormURLEncodedRequest() bool {
	return req.HasFormUrlEncoded(c.request)
}

func (c *ctx) HasJSONRequest() bool {
	return req.HasJSON(c.request)
}

func (c *ctx) IsInertiaRequest() bool {
	return inertia.IsInertiaRequest(c.request)
}

func (c *ctx) IsReading() bool {
	return c.request.Method == "GET" || c.request.Method == "HEAD" || c.request.Method == "OPTIONS"
}

func (c *ctx) Param(key string) string {
	return c.Request().PathValue(key)
}

func (c *ctx) Query(key string) string {
	return c.request.URL.Query().Get(key)
}

func (c *ctx) Form() (map[string][]string, error) {
	if c.request.Form != nil {
		return c.request.Form, nil
	}

	if c.HasMultiPartRequest() {
		if err := c.parseMultipartForm(); err != nil {
			return nil, err
		}
	}

	if c.HasFormURLEncodedRequest() {
		if err := c.request.ParseForm(); err != nil {
			return nil, err
		}
	}
	return c.request.Form, nil
}

func (c *ctx) Body() (map[string][]string, error) {
	if c.request.Form != nil {
		return c.request.Form, nil
	}

	if err := c.request.ParseForm(); err != nil {
		return nil, err
	}
	return c.request.Form, nil
}

func (c *ctx) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	if err := c.parseMultipartForm(); err != nil {
		return nil, nil, err
	}

	file, header, err := c.request.FormFile(key)
	if err != nil {
		c.cleanupMultipartForm()
		return nil, nil, err
	}
	if !c.multipartFormOwned {
		return file, header, nil
	}
	return &multipartFile{File: file, cleanup: c.cleanupMultipartForm}, header, nil
}

func (c *ctx) HasFile(key string) bool {
	if err := c.parseMultipartForm(); err != nil {
		return false
	}
	return c.request.MultipartForm != nil && len(c.request.MultipartForm.File[key]) > 0
}

func (c *ctx) parseMultipartForm() error {
	if c.request.MultipartForm != nil {
		return nil
	}
	if err := c.request.ParseMultipartForm(32 << 20); err != nil {
		return err
	}
	c.multipartFormOwned = true
	return nil
}

func (c *ctx) cleanupMultipartForm() {
	if !c.multipartFormOwned || c.request.MultipartForm == nil {
		return
	}
	c.multipartFormOnce.Do(func() {
		if err := c.request.MultipartForm.RemoveAll(); err != nil {
			slog.Info("Multipart form files could not be removed", "Error:", err)
		}
	})
}

type multipartFile struct {
	multipart.File
	cleanup func()
	once    sync.Once
	err     error
}

func (f *multipartFile) Close() error {
	f.once.Do(func() {
		f.err = f.File.Close()
		f.cleanup()
	})
	return f.err
}

func (c *ctx) Upload(uploadedFileName string, dir string, filename ...string) (*os.File, error) {
	file, header, err := c.FormFile(uploadedFileName)
	if err == nil {
		defer func() {
			if err := file.Close(); err != nil {
				slog.Info("Form file could not be closed", "Error:", err)
			}
		}()

		if len(filename) > 0 {
			header.Filename = filename[0]
		}

		fm := c.App().FileSystem()
		//fm := Get[*fs.FileSystem](c.App())
		//fm := fs.Get(c.App())
		if fm == nil {
			e := errors.New("FileManager not set")
			slog.Error(e.Error())
			return nil, e
		}

		fss, err := fm.Disk()

		if err != nil {
			return nil, err
		}

		return fss.Upload(file, header, dir)
	}

	if err != nil && !errors.Is(err, http.ErrMissingFile) {
		return nil, fmt.Errorf("could not get form file: %w", err)
	}
	return nil, errors.New("file with the provided uploadedFileName does not exist")
}

func (c *ctx) File(path string, headers ...map[string][]string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return c.Error(http.StatusNotFound, fmt.Errorf("file not found: %s", path))
	}

	file, err := os.Open(path)
	if err != nil {
		return c.Error(http.StatusInternalServerError, fmt.Errorf("could not open file: %w", err))
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Info("File could not be closed", "Error:", err)
		}
	}()

	c.writer.Header().Set("content-type", mime.TypeByExtension(filepath.Ext(file.Name())))
	c.writer.Header().Set("content-disposition", fmt.Sprintf("inline; filename=%s", filepath.Base(path)))

	if len(headers) > 0 {
		for key, values := range headers[0] {
			for _, value := range values {
				c.writer.Header().Set(key, value)
			}
		}
	}
	_, err = io.Copy(c.writer, file)
	return err
}

func (c *ctx) StorageFile(path string, headers ...map[string][]string) error {
	fm := c.App().FileSystem()
	//fm := fs.Get(c.App())
	if fm == nil {
		e := errors.New("FileManager not set")
		slog.Error(e.Error())
		return e
	}

	fss, err := fm.Disk()

	if err != nil {
		return err
	}
	if exists, err := fss.Exists(path); err != nil || !exists {
		return c.Error(http.StatusNotFound, fmt.Errorf("file not found: %s", path))
	}

	file, err := fss.Open(path)
	if err != nil {
		return c.Error(http.StatusInternalServerError, fmt.Errorf("could not open file: %w", err))
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Info("File could not be closed", "Error:", err)
		}
	}()

	c.writer.Header().Set("content-type", mime.TypeByExtension(filepath.Ext(file.Name())))
	c.writer.Header().Set("content-disposition", fmt.Sprintf("inline; filename=%s", filepath.Base(path)))

	if len(headers) > 0 {
		for key, values := range headers[0] {
			for _, value := range values {
				c.writer.Header().Set(key, value)
			}
		}
	}

	_, err = io.Copy(c.writer, file)
	return err
}

func (c *ctx) Download(path string, filename string) error {
	fm := c.App().FileSystem()
	//fm := fs.Get(c.App())
	if fm == nil {
		e := errors.New("FileManager not set")
		slog.Error(e.Error())
		return e
	}

	fss, err := fm.Disk()
	if err != nil {
		return err
	}

	if exists, err := fss.Exists(path); err != nil || !exists {
		return c.Error(http.StatusNotFound, fmt.Errorf("file not found: %s", path))
	}

	file, err := fss.Open(path)
	if err != nil {
		return c.Error(http.StatusInternalServerError, fmt.Errorf("could not open file: %w", err))
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Info("File could not be closed", "Error:", err)
		}
	}()

	c.writer.Header().Set("content-type", "application/octet-stream")
	c.writer.Header().Set("content-disposition", fmt.Sprintf("attachment; filename=%s", filename))
	_, err = io.Copy(c.writer, file)
	return err
}

func (c *ctx) SetRequest(r *http.Request) {
	c.Lock()
	defer c.Unlock()
	c.request = r
}

func (c *ctx) Set(key string, value any) {
	c.Lock()
	defer c.Unlock()
	c.request = c.request.WithContext(context.WithValue(c.request.Context(), key, value))
}

func (c *ctx) Get(key string) any {
	c.Lock()
	defer c.Unlock()
	return c.request.Context().Value(key)
}

func (c *ctx) PutSession(key string, value any) SessionGetSetter {
	sess, ok := Lookup[*session.Session](c.App())
	if !ok || sess == nil {
		e := errors.New("session not set")
		slog.Error(e.Error())
		return nil
	}

	sess.Put(c.Request().Context(), key, value)
	return c
}

func (c *ctx) PopSession(key string) any {
	sess, ok := Lookup[*session.Session](c.App())
	if !ok || sess == nil {
		e := errors.New("session not set")
		slog.Error(e.Error())
		return nil
	}

	return sess.Pop(c.Request().Context(), key)
}

func (c *ctx) PopSessionString(key string) string {
	sess, ok := Lookup[*session.Session](c.App())
	if !ok || sess == nil {
		e := errors.New("session not set")
		slog.Error(e.Error())
		return ""
	}

	return sess.PopString(c.Request().Context(), key)
}

func (c *ctx) Session(key string) any {
	sess, ok := Lookup[*session.Session](c.App())
	if !ok || sess == nil {
		e := errors.New("session not set")
		slog.Error(e.Error())
		return nil
	}

	return sess.Get(c.Request().Context(), key)
}

func (c *ctx) SessionString(key string) string {
	sess, ok := Lookup[*session.Session](c.App())
	if !ok || sess == nil {
		e := errors.New("session not set")
		slog.Error(e.Error())
		return ""
	}

	return sess.GetString(c.Request().Context(), key)
}

func (c *ctx) Error(status int, err error) error {
	if c.WantsJSON() {
		return c.SetStatus(status).JSON(M{"message": err.Error()})
	}
	c.writer.WriteHeader(status)
	if _, e := c.writer.Write([]byte(err.Error())); e != nil {
		return err
	}
	return nil
}

func (c *ctx) ValidationError(err error) error {
	var e shared.ValidationErrors

	if !errors.As(err, &e) {
		return c.Error(http.StatusInternalServerError, err)
	}

	// An Inertia request must receive an Inertia-shaped response. Deciding by
	// Accept or Referer gets this wrong: an XHR sends Accept: */*, which reads
	// as "wants JSON", so a failed form came back as a JSON body and the
	// client threw "All Inertia requests must receive a valid Inertia
	// response". The X-Inertia header is the reliable signal.
	if c.IsInertia() {
		c.PutSession("errors", e)
		return c.WithInput().Back()
	}

	if c.WantsJSON() || c.Referer() == "" {
		return c.SetStatus(http.StatusUnprocessableEntity).JSON(M{"errors": err})
	}

	c.PutSession("errors", e)

	return c.WithInput().Back()
}

// IsInertia reports whether the request came from an Inertia client.
func (c *ctx) IsInertia() bool {
	return c.Header("X-Inertia") != ""
}

func (c *ctx) InternalServerError(err ...error) error {
	if len(err) > 0 {
		return &InternalServerError{HttpMessage{http.StatusInternalServerError, fmt.Sprintf("server error: %s", err[0].Error())}}
	}
	return ErrInternalServerError
}

func (c *ctx) NotFound(err ...error) error {
	if len(err) > 0 {
		return &NotFoundError{HttpMessage{http.StatusNotFound, fmt.Sprintf("not found: %s", err[0].Error())}}
	}
	return ErrNotFound
}

func (c *ctx) BadRequest(err ...error) error {
	if len(err) > 0 {
		return &BadRequestError{HttpMessage{http.StatusBadRequest, fmt.Sprintf("bad request: %s", err[0].Error())}}
	}
	return ErrBadRequest
}

func (c *ctx) Unauthorized(err ...error) error {
	if len(err) > 0 {
		return &UnauthorizedError{HttpMessage{http.StatusUnauthorized, fmt.Sprintf("unauthorized: %s", err[0].Error())}}
	}
	return ErrUnauthorized
}

func (c *ctx) Forbidden(err ...error) error {
	if len(err) > 0 {
		return &ForbiddenError{HttpMessage{http.StatusForbidden, fmt.Sprintf("forbidden: %s", err[0].Error())}}
	}
	return ErrForbidden
}

func (c *ctx) PageExpired(err ...error) error {
	if len(err) > 0 {
		return &PageExpiredError{HttpMessage{419, fmt.Sprintf("page expired: %s", err[0].Error())}}
	}
	return ErrPageExpired
}

func (c *ctx) NoContent() error {
	c.SetStatus(http.StatusNoContent)
	c.writer.WriteHeader(http.StatusNoContent)
	return nil
}

func (c *ctx) MethodNotAllowed(err ...error) error {
	if len(err) > 0 {
		return &MethodNotAllowedError{HttpMessage{http.StatusMethodNotAllowed, fmt.Sprintf("method not allowed: %s", err[0].Error())}}
	}
	return ErrMethodNotAllowed
}

func (c *ctx) UnprocessableEntity(err ...error) error {
	if len(err) > 0 {
		return &UnprocessableEntityError{HttpMessage{http.StatusUnprocessableEntity, fmt.Sprintf("unprocessable entity: %s", err[0].Error())}}
	}
	return ErrUnprocessableEntity
}

func (c *ctx) DecodeJSON(v any) error {
	return req.DecodeJSONBody(c.writer, c.request, v)
}
