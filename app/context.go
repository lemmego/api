package app

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
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

	"github.com/lemmego/api/shared"
	inertia "github.com/romsar/gonertia"

	"github.com/lemmego/api/req"

	"github.com/a-h/templ"
)

func init() {
	gob.Register(shared.ValidationErrors{})
	gob.Register(shared.ValidationErrors{})
	gob.Register(map[string][]string{})
}

type Context interface {
	GetSetter
	HttpProvider
	App() App
	Next() error
}

type GetSetter interface {
	Get(key string) any
	Set(key string, value any)
}

type RequestGetSetter interface {
	Request() *http.Request
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
	HasFormURLEncodedRequest() bool
}

type AcceptHeaderResolver interface {
	WantsJSON() bool
	WantsHTML() bool
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
	InternalServerError(err error) error
	NotFound(err error) error
	BadRequest(err error) error
	Unauthorized(err error) error
	Forbidden(err error) error
	PageExpired() error
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

//type R struct {
//	Status       int
//	Payload      M
//	Renderer     res.Renderer
//	TemplateView string
//	TemplView    string
//	InertiaView  string
//	RedirectTo   string
//}

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

//func (c *ctx) Alert(typ string, message string) *res.AlertMessage {
//	if typ != "success" && typ != "error" && typ != "warning" && typ != "info" && typ != "debug" {
//		return &res.AlertMessage{Type: "", Body: ""}
//	}
//
//	return &res.AlertMessage{Type: typ, Body: message}
//}

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

	v := reflect.ValueOf(inputStruct).Elem()

	nameField := v.FieldByName("BaseInput")
	if nameField.IsValid() && nameField.CanSet() {
		i := &BaseInput{Validator: NewValidator(c.app), app: c.app, ctx: c}
		nameField.Set(reflect.ValueOf(i))
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

//func (c *ctx) Respond(r *R) error {
//	if r.RedirectTo != "" {
//		if r.Status == 0 {
//			r.Status = http.StatusFound
//		}
//		return c.Status(r.Status).Redirect(r.RedirectTo)
//	}

//if r.InertiaView != "" {
//	if r.Status == 0 {
//		r.Status = http.StatusOK
//	}
//	return c.Status(r.Status).Inertia(r.InertiaView, r.Payload)
//}
//
//if r.TemplateView != "" {
//	templateData := &res.TemplateOpts{}
//
//	if r.Payload != nil {
//		templateData.Data = r.Payload
//	}
//	return c.Render(r.TemplateView, templateData)
//}

//	if r.Payload != nil {
//		if r.Status == 0 {
//			r.Status = http.StatusOK
//		}
//		return c.Status(r.Status).JSON(r.Payload)
//	}
//
//	return nil
//}

func (c *ctx) Render(r Renderer) error {
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

func (c *ctx) JSON(body M) error {
	// TODO: Check if header is already sent
	response, _ := json.Marshal(body)
	c.writer.Header().Set("content-Type", "application/json")
	if c.status == 0 {
		c.status = http.StatusOK
	}
	c.writer.WriteHeader(c.status)
	_, err := c.writer.Write(response)
	return err
}

func (c *ctx) AuthUser(sessKey string) interface{} {
	return c.PopSession(sessKey)
}

//func (c *ctx) resolveTemplateData(data *res.TemplateOpts) *res.TemplateOpts {
//	if data == nil {
//		data = &res.TemplateOpts{}
//	}
//
//	vErrs := shared.ValidationErrors{}
//
//	if val, ok := c.PopSession("errors").(shared.ValidationErrors); ok {
//		vErrs = val
//	}
//
//	if data.ValidationErrors == nil {
//		data.ValidationErrors = vErrs
//	}
//
//	data.Messages = append(data.Messages, &res.AlertMessage{"success", c.PopSessionString("success")})
//	data.Messages = append(data.Messages, &res.AlertMessage{"info", c.PopSessionString("info")})
//	data.Messages = append(data.Messages, &res.AlertMessage{"warning", c.PopSessionString("warning")})
//	data.Messages = append(data.Messages, &res.AlertMessage{"error", c.PopSessionString("error")})
//
//	return data
//}

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

//func (c *ctx) Render(tmplPath string, data *res.TemplateOpts) error {
//	data = c.resolveTemplateData(data)
//	c.writer.Header().Set("content-type", "text/html")
//	if c.status == 0 {
//		c.status = http.StatusOK
//	}
//	c.writer.WriteHeader(c.status)
//	data.funcMap = template.funcMap{
//		"csrf": func() template.HTML {
//			token := c.SessionString("_token")
//			return template.HTML(`<input type="hidden" name="_token" value="` + token + `" />`)
//		},
//	}
//	return res.RenderTemplate(c.writer, tmplPath, data)
//}

//func (c *ctx) Inertia(filePath string, props map[string]any) error {
//	i, err := di.Resolve[*inertia.Inertia](c.App().Container())
//	if i == nil || err != nil {
//		return fmt.Errorf("inertia not enabled: %w", err)
//	}
//
//	if errs := c.PopSession("errors"); errs != nil {
//		if props == nil {
//			props = map[string]any{}
//		}
//
//		props["errors"] = errs
//	}
//
//	if c.status == 0 {
//		c.status = http.StatusOK
//	}
//	c.writer.WriteHeader(c.status)
//	return i.Render(c.ResponseWriter(), c.Request(), filePath, props)
//}

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
	return c.Redirect(c.Referer())
}

func (c *ctx) Referer() string {
	return c.request.Referer()
}

func (c *ctx) HasMultiPartRequest() bool {
	contentType := strings.ToLower(c.Header("Content-Type"))
	return contentType != "" && strings.HasPrefix(contentType, "multipart/")
}

func (c *ctx) HasFormURLEncodedRequest() bool {
	contentType := strings.ToLower(c.Header("Content-Type"))
	return contentType == "application/x-www-form-urlencoded"
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

	var err error

	if c.HasMultiPartRequest() {
		err = c.request.ParseMultipartForm(32 << 20)
	}

	if c.HasFormURLEncodedRequest() {
		err = c.request.ParseForm()
	}

	if err != nil {
		return nil, err
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
	if file, _, err := c.request.FormFile(key); file != nil && err == nil {
		return c.request.FormFile(key)
	}

	if err := c.request.ParseMultipartForm(32 << 20); err != nil {
		return nil, nil, err
	}
	return c.request.FormFile(key)
}

func (c *ctx) HasFile(key string) bool {
	_, _, err := c.request.FormFile(key)
	return err == nil
}

func (c *ctx) Upload(uploadedFileName string, dir string, filename ...string) (*os.File, error) {
	if c.HasFile(uploadedFileName) {
		file, header, err := c.FormFile(uploadedFileName)

		if err != nil {
			return nil, fmt.Errorf("could not get form file: %w", err)
		}

		defer func() {
			err := file.Close()
			if err != nil {
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

	return nil, errors.New("file with the provided uploadedFileName does not exist")
}

func (c *ctx) File(path string, headers ...map[string][]string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return c.Error(http.StatusNotFound, fmt.Errorf("file not found: %s", path))
	}

	file, err := os.Open(path)
	defer func() {
		err := file.Close()
		if err != nil {
			slog.Info("File could not be closed", "Error:", err)
		}
	}()

	if err != nil {
		return c.Error(http.StatusInternalServerError, fmt.Errorf("could not open file: %w", err))
	}

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
	defer func() {
		err := file.Close()
		if err != nil {
			slog.Info("File could not be closed", "Error:", err)
		}
	}()

	if err != nil {
		return c.Error(http.StatusInternalServerError, fmt.Errorf("could not open file: %w", err))
	}

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
	defer func() {
		err := file.Close()
		if err != nil {
			slog.Info("File could not be closed", "Error:", err)
		}
	}()

	if err != nil {
		return c.Error(http.StatusInternalServerError, fmt.Errorf("could not open file: %w", err))
	}

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

func (c *ctx) Set(key string, value interface{}) {
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
	sess := c.App().Session()
	//sess := session.Get(c.app)

	if sess == nil {
		e := errors.New("session not set")
		slog.Error(e.Error())
		return nil
	}

	sess.Put(c.Request().Context(), key, value)
	return c
}

func (c *ctx) PopSession(key string) any {
	sess := c.App().Session()
	//sess := session.Get(c.app)

	if sess == nil {
		e := errors.New("session not set")
		slog.Error(e.Error())
		return nil
	}

	return sess.Pop(c.Request().Context(), key)
}

func (c *ctx) PopSessionString(key string) string {
	sess := c.App().Session()
	//sess := session.Get(c.app)

	if sess == nil {
		e := errors.New("session not set")
		slog.Error(e.Error())
		return ""
	}

	return sess.PopString(c.Request().Context(), key)
}

func (c *ctx) Session(key string) any {
	sess := c.App().Session()
	//sess := session.Get(c.app)

	if sess == nil {
		e := errors.New("session not set")
		slog.Error(e.Error())
		return nil
	}

	return sess.Get(c.Request().Context(), key)
}

func (c *ctx) SessionString(key string) string {
	sess := c.App().Session()
	//sess := session.Get(c.app)

	if sess == nil {
		e := errors.New("session not set")
		slog.Error(e.Error())
		return ""
	}

	return sess.GetString(c.Request().Context(), key)
}

func (c *ctx) Error(status int, err error) error {
	if c.WantsJSON() {
		return c.JSON(M{"message": err.Error()})
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

	if c.WantsJSON() || c.Referer() == "" {
		return c.SetStatus(http.StatusUnprocessableEntity).JSON(M{"errors": err})
	}

	c.PutSession("errors", err.(shared.ValidationErrors))

	return c.WithInput().Back()
}

func (c *ctx) InternalServerError(err error) error {
	return c.Error(http.StatusInternalServerError, err)
}

func (c *ctx) NotFound(err error) error {
	return c.Error(http.StatusNotFound, err)
}

func (c *ctx) BadRequest(err error) error {
	return c.Error(http.StatusBadRequest, err)
}

func (c *ctx) Unauthorized(err error) error {
	return c.Error(http.StatusUnauthorized, err)
}

func (c *ctx) Forbidden(err error) error {
	return c.Error(http.StatusForbidden, err)
}

func (c *ctx) PageExpired() error {
	return c.Error(419, errors.New("page expired"))
}

func (c *ctx) NoContent() error {
	_, err := c.SetStatus(204).Write(nil)
	if err != nil {
		return c.Error(http.StatusInternalServerError, err)
	}
	return nil
}

func (c *ctx) DecodeJSON(v interface{}) error {
	return req.DecodeJSONBody(c.writer, c.request, v)
}
