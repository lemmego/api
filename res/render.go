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

	// Find all layout templates (glob pattern for nested directories)
	layouts, err := filepath.Glob("./templates/**/*.layout.tmpl")
	if err != nil {
		return myCache, fmt.Errorf("error finding layout templates: %v", err)
	}

	// Walk through the template directory to find all .page.tmpl files
	err = filepath.Walk("./templates", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories, only process .tmpl files
		if !info.IsDir() && filepath.Ext(path) == ".tmpl" && strings.HasSuffix(path, ".page.tmpl") {
			relPath, err := filepath.Rel("./templates", path) // Get relative path from ./templates
			if err != nil {
				return err
			}

			// Create a new template set and parse the page template
			ts := template.New(filepath.Base(path))
			ts, err = ts.ParseFiles(path)
			if err != nil {
				return fmt.Errorf("error parsing template %s: %v", relPath, err)
			}

			// Parse all found layout templates into the same template set
			if len(layouts) > 0 {
				ts, err = ts.ParseFiles(layouts...)
				if err != nil {
					return fmt.Errorf("error parsing layout templates for %s: %v", relPath, err)
				}
			}

			// Cache the template using the relative path (e.g., "foo/bar.page.tmpl")
			myCache[relPath] = ts
		}

		return nil
	})

	if err != nil {
		return myCache, fmt.Errorf("error walking template directory: %v", err)
	}

	return myCache, nil
}
