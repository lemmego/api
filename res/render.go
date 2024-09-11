package res

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"

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
	CSRFToken        string
	ValidationErrors shared.ValidationErrors
	Messages         []*AlertMessage
}

func init() {
	// Initialize template cache once during startup
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

	// Apply FuncMap here
	if data.FuncMap != nil {
		t = t.Funcs(data.FuncMap)
	}

	return t.Execute(w, data)
}

func createTemplateCache() (map[string]*template.Template, error) {
	myCache := map[string]*template.Template{}

	pages, err := filepath.Glob("./templates/*.page.tmpl")
	if err != nil {
		return myCache, fmt.Errorf("error finding page templates: %v", err)
	}

	partials, err := filepath.Glob("./templates/*.partial.tmpl")
	if err != nil {
		return myCache, fmt.Errorf("error finding partial templates: %v", err)
	}

	for _, page := range append(pages, partials...) {
		name := filepath.Base(page)
		ts := template.New(name)

		ts, err := ts.ParseFiles(page)
		if err != nil {
			return myCache, fmt.Errorf("error parsing page template %s: %v", name, err)
		}

		ts, err = ts.ParseGlob("./templates/*.layout.tmpl")
		if err != nil {
			return myCache, fmt.Errorf("error parsing layout templates for %s: %v", name, err)
		}

		myCache[name] = ts
	}

	return myCache, nil
}
