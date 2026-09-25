package res

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lemmego/api/app"
	"github.com/lemmego/api/shared"
)

var (
	templateCacheMu sync.RWMutex
	templateCache   map[string]*template.Template
)

// Renderer defines the interface for types that can render content.
// csrfTokenKey is where the CSRF middleware stores its token, on both the
// request context and the template data.
const csrfTokenKey = "_token"

type Renderer interface {
	Render(w io.Writer) error
}

type Template struct {
	File             string
	stringMap        map[string]string
	intMap           map[string]int
	floatMap         map[string]float64
	boolMap          map[string]bool
	funcMap          template.FuncMap
	data             map[string]any
	validationErrors shared.ValidationErrors
	ctx              app.Context
}

func NewTemplate(ctx app.Context, fileName string) *Template {
	return &Template{File: fileName, ctx: ctx}
}

func (t *Template) WithData(data map[string]any) *Template {
	t.data = data
	return t
}

func (t *Template) WithFloatMap(floatMap map[string]float64) *Template {
	t.floatMap = floatMap
	return t
}

func (t *Template) WithIntMap(intMap map[string]int) *Template {
	t.intMap = intMap
	return t
}

func (t *Template) WithBoolMap(boolMap map[string]bool) *Template {
	t.boolMap = boolMap
	return t
}

func (t *Template) WithFuncMap(funcMap template.FuncMap) *Template {
	t.funcMap = funcMap
	return t
}

func (t *Template) WithValidationErrors(validationErrors shared.ValidationErrors) *Template {
	t.validationErrors = validationErrors
	return t
}

func (t *Template) Render(w io.Writer) error {
	templateCacheMu.RLock()
	tmpl, ok := templateCache[t.File]
	templateCacheMu.RUnlock()
	if !ok {
		return fmt.Errorf("template %s not found in cache", t.File)
	}
	if t.funcMap != nil {
		var err error
		tmpl, err = tmpl.Clone()
		if err != nil {
			return fmt.Errorf("clone template %s: %w", t.File, err)
		}
		tmpl = tmpl.Funcs(t.funcMap)
	}
	vErrs := shared.ValidationErrors{}

	if t.ctx != nil {
		if val, ok := t.ctx.PopSession("errors").(shared.ValidationErrors); ok {
			vErrs = val
		}
	}

	validationErrors := t.validationErrors
	if validationErrors == nil {
		validationErrors = vErrs
	}

	data := make(map[string]any, len(t.data)+2)
	for key, value := range t.data {
		data[key] = value
	}
	data["errors"] = validationErrors

	// The CSRF middleware puts the token on the request context, and every
	// form that posts needs it in a hidden _token field. Passing it through by
	// hand in each handler is easy to forget, and forgetting it produces a 419
	// at submit time rather than anything visible while building the page.
	// An explicit value wins, so a caller can still override it.
	if _, ok := data[csrfTokenKey]; !ok && t.ctx != nil {
		if token, ok := t.ctx.Get(csrfTokenKey).(string); ok && token != "" {
			data[csrfTokenKey] = token
		}
	}

	return tmpl.Execute(w, data)
}

// LoadTemplates parses page templates below dir and replaces the current cache.
// It is explicit so importing this package never depends on the process working
// directory or terminates the process when templates are unavailable.
func LoadTemplates(dir string) error {
	cache, err := createTemplateCache(dir)
	if err != nil {
		return err
	}

	templateCacheMu.Lock()
	templateCache = cache
	templateCacheMu.Unlock()
	return nil
}

//func RenderTemplate(w http.ResponseWriter, tmpl string, data *TemplateOpts) error {
//	t, ok := templateCache[tmpl]
//	if !ok {
//		return fmt.Errorf("template %s not found in cache", tmpl)
//	}
//	if data.funcMap != nil {
//		t = t.Funcs(data.funcMap)
//	}
//	return t.Execute(w, data)
//}

func createTemplateCache(dir string) (map[string]*template.Template, error) {
	myCache := map[string]*template.Template{}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".page.gohtml") {
			return nil
		}

		name, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("error getting relative path: %v", err)
		}

		ts, err := template.New(filepath.Base(path)).Funcs(template.FuncMap{"csrf": func() template.HTML { return "" }}).ParseFiles(path)
		if err != nil {
			return fmt.Errorf("error parsing page template %s: %v", name, err)
		}

		// Find and parse layout templates
		layouts, err := findTemplates(filepath.Dir(path), "*.layout.gohtml", dir)
		if err != nil {
			return fmt.Errorf("error finding layout templates for %s: %v", name, err)
		}

		// Find and parse partial templates
		partials, err := findTemplates(filepath.Dir(path), "*.partial.gohtml", dir)
		if err != nil {
			return fmt.Errorf("error finding partial templates for %s: %v", name, err)
		}

		// Combine layouts and partials
		templatestoAdd := append(layouts, partials...)

		if len(templatestoAdd) > 0 {
			ts, err = ts.ParseFiles(templatestoAdd...)
			if err != nil {
				return fmt.Errorf("error parsing additional templates for %s: %v", name, err)
			}
		}

		myCache[name] = ts
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking templates directory: %v", err)
	}

	return myCache, nil
}

func findTemplates(dir, pattern, root string) ([]string, error) {
	var templates []string
	for {
		files, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, fmt.Errorf("error searching for templates in %s: %v", dir, err)
		}
		templates = append(templates, files...)
		if filepath.Clean(dir) == filepath.Clean(root) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return templates, nil
}
