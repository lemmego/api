package res

import (
	"fmt"
	"github.com/lemmego/api/app"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/lemmego/api/shared"
)

var templateCache map[string]*template.Template

// Renderer defines the interface for types that can render content.
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
	tmpl, ok := templateCache[t.File]
	if !ok {
		return fmt.Errorf("template %s not found in cache", t.File)
	}
	if t.funcMap != nil {
		tmpl = tmpl.Funcs(t.funcMap)
	}
	vErrs := shared.ValidationErrors{}

	if val, ok := t.ctx.PopSession("errors").(shared.ValidationErrors); ok {
		vErrs = val
	}

	if t.validationErrors == nil {
		t.validationErrors = vErrs
	}

	return tmpl.Execute(w, t)
}

func init() {
	var err error
	templateCache, err = createTemplateCache()
	if err != nil {
		log.Fatalf("failed to create template cache: %v", err)
	}
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

func createTemplateCache() (map[string]*template.Template, error) {
	myCache := map[string]*template.Template{}

	err := filepath.Walk("./templates", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".page.gohtml") {
			return nil
		}

		name, err := filepath.Rel("./templates", path)
		if err != nil {
			return fmt.Errorf("error getting relative path: %v", err)
		}

		ts, err := template.New(filepath.Base(path)).Funcs(template.FuncMap{"csrf": func() template.HTML { return "" }}).ParseFiles(path)
		if err != nil {
			return fmt.Errorf("error parsing page template %s: %v", name, err)
		}

		// Find and parse layout templates
		layouts, err := findTemplates(filepath.Dir(path), "*.layout.gohtml")
		if err != nil {
			return fmt.Errorf("error finding layout templates for %s: %v", name, err)
		}

		// Find and parse partial templates
		partials, err := findTemplates(filepath.Dir(path), "*.partial.gohtml")
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

func findTemplates(dir, pattern string) ([]string, error) {
	var templates []string
	for dir != "." && dir != "/" {
		files, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, fmt.Errorf("error searching for templates in %s: %v", dir, err)
		}
		templates = append(templates, files...)
		dir = filepath.Dir(dir)
	}
	return templates, nil
}
