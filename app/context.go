package app

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/lemmego/api/db"
	"github.com/lemmego/api/fsys"
	"github.com/lemmego/api/logger"
	"github.com/lemmego/api/res"
	"github.com/lemmego/api/shared"

	inertia "github.com/romsar/gonertia"

	"github.com/lemmego/api/req"

	"github.com/a-h/templ"
)

func init() {
	gob.Register(&res.AlertMessage{})
	gob.Register(shared.ValidationErrors{})
	gob.Register([]*res.AlertMessage{})
	gob.Register(shared.ValidationErrors{})
	gob.Register(map[string][]string{})
}

type Context struct {
	sync.Mutex
	app     AppManager
	request *http.Request
	writer  http.ResponseWriter

	handlers []Handler
	index    int
}

type R struct {
	Status       int
	TemplateName string
	InertiaView  string
	Payload      M
	RedirectTo   string
}

func (c *Context) Plugin(plugin Plugin) Plugin {
	return c.App().Plugin(plugin)
}

func (c *Context) Next() error {
	c.index++
	if c.index < len(c.handlers) {
		return c.handlers[c.index](c)
	}
	return nil
}

// SetCookie sets a cookie on the response writer
// Example: // c.SetCookie("jwt", token, 60*60*24*7, "/", "", false, true)
func (c *Context) SetCookie(name string, value string, maxAge int, path string, domain string, secure bool, httpOnly bool) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   maxAge,
		Path:     path,
		Domain:   domain,
		Secure:   secure,
		HttpOnly: httpOnly,
	}
	http.SetCookie(c.writer, cookie)
}

func (c *Context) Alert(typ string, message string) *res.AlertMessage {
	if typ != "success" && typ != "error" && typ != "warning" && typ != "info" && typ != "debug" {
		return &res.AlertMessage{Type: "", Body: ""}
	}

	return &res.AlertMessage{Type: typ, Body: message}
}

func (c *Context) Validate(body req.Validator) error {
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

func (c *Context) ParseInput(inputStruct any) error {
	err := req.ParseInput(c, inputStruct)
	if err != nil {
		return err
	}

	v := reflect.ValueOf(inputStruct).Elem()

	nameField := v.FieldByName("BaseInput")
	if nameField.IsValid() && nameField.CanSet() {
		i := &BaseInput{App: c.app, Ctx: c, Validator: NewValidator(c.app)}
		nameField.Set(reflect.ValueOf(i))
	}
	return nil
}

func (c *Context) Input(inputStruct any) any {
	err := req.In(c, inputStruct)
	if err != nil {
		return nil
	}
	return c.Get(HTTPInKey)
}

func (c *Context) SetInput(inputStruct any) error {
	err := req.In(c, inputStruct)
	if err != nil {
		return err
	}
	return nil
}

func (c *Context) GetInput() any {
	return c.Get(HTTPInKey)
}

func (c *Context) Respond(r *R) error {
	if r.Status == 0 {
		r.Status = http.StatusOK
	}

	if c.WantsJSON() {
		if r.Payload != nil {
			return c.JSON(r.Status, r.Payload)
		}
	}

	if r.InertiaView != "" {
		return c.Inertia(r.Status, r.InertiaView, r.Payload)
	}

	if r.RedirectTo != "" {
		return c.Redirect(http.StatusFound, r.RedirectTo)
	}

	templateData := &res.TemplateData{}

	if r.Payload != nil {
		templateData.Data = r.Payload
	}

	if r.TemplateName != "" {
		return c.Render(r.Status, r.TemplateName, templateData)
	}

	return nil
}

func (c *Context) App() AppManager {
	return c.app
}

func (c *Context) Request() *http.Request {
	return c.request
}

func (c *Context) ResponseWriter() http.ResponseWriter {
	return c.writer
}

func (c *Context) RequestContext() context.Context {
	return c.request.Context()
}

func (c *Context) FS() fsys.FS {
	return c.app.FS()
}

func (c *Context) DB() *db.DB {
	c.app.DbFunc(c.request.Context(), nil)
	return c.app.DB()
	//dbm, err := c.app.Resolve((*db.DB)(nil))
	//if err != nil {
	//	log.Println(fmt.Errorf("db: %w", err))
	//	return nil
	//}
	//
	//return dbm.(*db.DB)
}

func (c *Context) I() *inertia.Inertia {
	return c.app.Inertia()
}

func (c *Context) Templ(component templ.Component) error {
	return component.Render(c.Request().Context(), c.writer)
}

func (c *Context) GetHeader(key string) string {
	return c.request.Header.Get(key)
}

func (c *Context) SetHeader(key string, value string) {
	c.writer.Header().Add(key, value)
}

func (c *Context) WantsJSON() bool {
	return req.WantsJSON(c.request)
}

func (c *Context) JSON(status int, body M) error {
	// TODO: Check if header is already sent
	response, _ := json.Marshal(body)
	c.writer.Header().Set("content-Type", "application/json")
	c.writer.WriteHeader(status)
	_, err := c.writer.Write(response)
	return err
}

func (c *Context) AuthUser() interface{} {
	return c.PopSession("authUser")
}

func (c *Context) resolveTemplateData(data *res.TemplateData) *res.TemplateData {
	if data == nil {
		data = &res.TemplateData{}
	}

	vErrs := shared.ValidationErrors{}

	if val, ok := c.PopSession("errors").(shared.ValidationErrors); ok {
		vErrs = val
	}

	if data.ValidationErrors == nil {
		data.ValidationErrors = vErrs
	}

	data.Messages = append(data.Messages, &res.AlertMessage{"success", c.PopSessionString("success")})
	data.Messages = append(data.Messages, &res.AlertMessage{"info", c.PopSessionString("info")})
	data.Messages = append(data.Messages, &res.AlertMessage{"warning", c.PopSessionString("warning")})
	data.Messages = append(data.Messages, &res.AlertMessage{"error", c.PopSessionString("error")})

	return data
}

func (c *Context) Text(status int, body []byte) error {
	c.writer.Header().Set("content-type", "text/plain")
	c.writer.WriteHeader(status)
	_, err := c.writer.Write(body)
	return err
}

func (c *Context) HTML(status int, body []byte) error {
	c.writer.Header().Set("content-type", "text/html")
	c.writer.WriteHeader(status)
	_, err := c.writer.Write(body)
	return err
}

func (c *Context) Render(status int, tmplPath string, data *res.TemplateData) error {
	data = c.resolveTemplateData(data)
	c.writer.Header().Set("content-type", "text/html")
	c.writer.WriteHeader(status)
	return res.RenderTemplate(c.writer, tmplPath, data)
}

func (c *Context) Inertia(status int, filePath string, props map[string]any) error {
	if c.app.Inertia() == nil {
		return errors.New("inertia not enabled")
	}

	if errs := c.PopSession("errors"); errs != nil {
		if props == nil {
			props = map[string]any{}
		}

		props["errors"] = errs
	}

	if input := c.PopSession("input"); input != nil {
		if props == nil {
			props = map[string]any{}
		}

		props["input"] = input
	}

	c.writer.WriteHeader(status)
	return c.App().Inertia().Render(c.ResponseWriter(), c.Request(), filePath, props)
}

func (c *Context) Redirect(status int, url string) error {
	if c.I() != nil {
		c.I().Redirect(c.ResponseWriter(), c.Request(), url)
		return nil
	}

	c.writer.Header().Set("Location", url)
	c.writer.WriteHeader(status)
	return nil
}

func (c *Context) With(key string, message string) *Context {
	return c.PutSession(key, message)
}

func (c *Context) WithErrors(errors shared.ValidationErrors) *Context {
	return c.PutSession("errors", errors)
}

func (c *Context) WithSuccess(message string) *Context {
	return c.PutSession("success", message)
}

func (c *Context) WithInfo(message string) *Context {
	return c.PutSession("info", message)
}

func (c *Context) WithWarning(message string) *Context {
	return c.PutSession("warning", message)
}

func (c *Context) WithError(message string) *Context {
	return c.PutSession("error", message)
}

func (c *Context) WithData(data map[string]any) *Context {
	return c.PutSession("data", data)
}

func (c *Context) WithInput() *Context {
	body, err := c.Form()
	if err == nil && body != nil {
		c.PutSession("input", body)
	}
	return c
}

func (c *Context) Back(status int) error {
	if c.app.Inertia() != nil {
		c.App().Inertia().Back(c.ResponseWriter(), c.Request(), status)
		return nil
	}

	return c.Redirect(status, c.Referer())
}

func (c *Context) Referer() string {
	return c.request.Referer()
}

func (c *Context) HasMultiPartRequest() bool {
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	return contentType != "" && strings.HasPrefix(contentType, "multipart/")
}

func (c *Context) HasFormURLEncodedRequest() bool {
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	return contentType == "application/x-www-form-urlencoded"
}

func (c *Context) IsInertiaRequest() bool {
	return inertia.IsInertiaRequest(c.request)
}

func (c *Context) Param(key string) string {
	return c.Request().PathValue(key)
}

func (c *Context) Query(key string) string {
	return c.request.URL.Query().Get(key)
}

func (c *Context) Form() (map[string][]string, error) {
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

func (c *Context) Body() (map[string][]string, error) {
	if c.request.Form != nil {
		return c.request.Form, nil
	}

	if err := c.request.ParseForm(); err != nil {
		return nil, err
	}
	return c.request.Form, nil
}

func (c *Context) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	if file, _, err := c.request.FormFile(key); file != nil && err == nil {
		return c.request.FormFile(key)
	}

	if err := c.request.ParseMultipartForm(32 << 20); err != nil {
		return nil, nil, err
	}
	return c.request.FormFile(key)
}

func (c *Context) HasFile(key string) bool {
	_, _, err := c.request.FormFile(key)
	return err == nil
}

func (c *Context) Upload(key string, dir string, filename ...string) (*os.File, error) {
	if c.HasFile(key) {
		file, header, err := c.FormFile(key)

		if err != nil {
			return nil, fmt.Errorf("could not get form file: %w", err)
		}

		defer func() {
			err := file.Close()
			if err != nil {
				logger.V().Info("Form file could not be closed", "Error:", err)
			}
		}()

		if len(filename) > 0 {
			header.Filename = filename[0]
		}

		return c.FS().Upload(file, header, dir)
	}

	return nil, nil
}

func (c *Context) Set(key string, value interface{}) {
	c.Lock()
	defer c.Unlock()
	c.request = c.request.WithContext(context.WithValue(c.request.Context(), key, value))
}

func (c *Context) SetRequest(r *http.Request) {
	c.Lock()
	defer c.Unlock()
	c.request = r
}

func (c *Context) Get(key string) any {
	c.Lock()
	defer c.Unlock()
	return c.request.Context().Value(key)
}

func (c *Context) PutSession(key string, value any) *Context {
	c.app.Session().Put(c.Request().Context(), key, value)
	return c
}

func (c *Context) PopSession(key string) any {
	return c.app.Session().Pop(c.Request().Context(), key)
}

func (c *Context) PopSessionString(key string) string {
	return c.app.Session().PopString(c.Request().Context(), key)
}

func (c *Context) GetSession(key string) any {
	return c.app.Session().Get(c.Request().Context(), key)
}

func (c *Context) GetSessionString(key string) string {
	return c.app.Session().GetString(c.Request().Context(), key)
}

func (c *Context) Error(status int, err error) error {
	if c.WantsJSON() {
		return c.JSON(status, M{"message": err.Error()})
	}
	c.writer.WriteHeader(status)
	if _, e := c.writer.Write([]byte(err.Error())); e != nil {
		return err
	}
	return err
}

func (c *Context) ValidationError(err error) error {
	var e shared.ValidationErrors

	if !errors.As(err, &e) {
		return c.Error(http.StatusInternalServerError, err)
	}

	if c.WantsJSON() || c.Referer() == "" {
		return c.JSON(http.StatusUnprocessableEntity, M{"errors": err})
	}

	c.WithErrors(err.(shared.ValidationErrors)).WithInput().Back(http.StatusFound)
	return nil
}

func (c *Context) InternalServerError(err error) error {
	return c.Error(http.StatusInternalServerError, err)
}

func (c *Context) NotFound(err error) error {
	return c.Error(http.StatusNotFound, err)
}

func (c *Context) BadRequest(err error) error {
	return c.Error(http.StatusBadRequest, err)
}

func (c *Context) Unauthorized(err error) error {
	return c.Error(http.StatusUnauthorized, err)
}

func (c *Context) Forbidden(err error) error {
	return c.Error(http.StatusForbidden, err)
}

func (c *Context) DecodeJSON(v interface{}) error {
	return req.DecodeJSONBody(c.writer, c.request, v)
}
