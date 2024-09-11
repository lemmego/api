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

func createTemplateCache() (map[string]*template.Template, error) {
	myCache := map[string]*template.Template{}

	// Find all .tmpl files (including in nested directories)
	templates, err := filepath.Glob("./templates/**/*.tmpl")
	if err != nil {
		return myCache, fmt.Errorf("error finding templates: %v", err)
	}

	// Loop through all the template files
	for _, tmpl := range templates {
		// Extract the file name (e.g., bar.tmpl)
		name := filepath.Base(tmpl)

		// Create a new template set for each file
		ts := template.New(name)

		// Parse all templates into the same template set (including the current template)
		ts, err = ts.ParseFiles(templates...)
		if err != nil {
			return myCache, fmt.Errorf("error parsing templates for %s: %v", tmpl, err)
		}

		// Store the parsed template set in the cache, using the relative path of the template
		myCache[tmpl] = ts
	}

	return myCache, nil
}

func RenderTemplate(w http.ResponseWriter, tmpl string, data *TemplateData) error {
	// Look up the requested template from the cache
	t, ok := templateCache[tmpl]
	if !ok {
		return fmt.Errorf("template %s not found in cache", tmpl)
	}

	// Apply the FuncMap if provided
	if data.FuncMap != nil {
		t = t.Funcs(data.FuncMap)
	}

	// Execute the template with the provided data
	return t.Execute(w, data)
}
