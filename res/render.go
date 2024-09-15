package res

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/lemmego/api/shared"
)

var templateCache map[string]*template.Template

type AlertMessage struct {
	Type string // success, error, warning, info, debug
	Body string
}

type TemplateData struct {
	StringMap        map[string]string
	IntMap           map[string]int
	FloatMap         map[string]float64
	BoolMap          map[string]bool
	FuncMap          template.FuncMap
	Data             map[string]any
	ValidationErrors shared.ValidationErrors
	Messages         []*AlertMessage
}

func init() {
	var err error
	templateCache, err = createTemplateCache()
	if err != nil {
		log.Fatalf("failed to create template cache: %v", err)
	}
}

func RenderTemplate(w http.ResponseWriter, tmpl string, data *TemplateData) error {
	t, ok := templateCache[tmpl]
	if !ok {
		return fmt.Errorf("template %s not found in cache", tmpl)
	}
	if data.FuncMap != nil {
		t = t.Funcs(data.FuncMap)
	}
	return t.Execute(w, data)
}

func createTemplateCache() (map[string]*template.Template, error) {
	myCache := map[string]*template.Template{}

	err := filepath.Walk("./templates", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".page.tmpl") {
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
		layouts, err := findTemplates(filepath.Dir(path), "*.layout.tmpl")
		if err != nil {
			return fmt.Errorf("error finding layout templates for %s: %v", name, err)
		}

		// Find and parse partial templates
		partials, err := findTemplates(filepath.Dir(path), "*.partial.tmpl")
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
