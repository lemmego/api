package res

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
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

	// Find all layout templates
	layouts, err := filepath.Glob("./templates/**/*.layout.tmpl")
	if err != nil {
		return myCache, fmt.Errorf("error finding layout templates: %v", err)
	}

	// Collect all page and partial template files
	err = filepath.Walk("./templates", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories, only process files with .tmpl extension
		if !info.IsDir() && (filepath.Ext(path) == ".tmpl") {
			// Extract the relative path (from ./templates)
			relPath, err := filepath.Rel("./templates", path)
			if err != nil {
				return err
			}

			// Create a new template set and parse the template file
			ts := template.New(filepath.Base(path))

			ts, err = ts.ParseFiles(path)
			if err != nil {
				return fmt.Errorf("error parsing template %s: %v", relPath, err)
			}

			// Only parse layout templates if there are any
			if len(layouts) > 0 {
				ts, err = ts.ParseGlob("./templates/**/*.layout.tmpl")
				if err != nil {
					return fmt.Errorf("error parsing layout templates for %s: %v", relPath, err)
				}
			}

			// Store the template in the cache, using the relative path as the key
			myCache[relPath] = ts
		}

		return nil
	})

	if err != nil {
		return myCache, fmt.Errorf("error walking template directory: %v", err)
	}

	return myCache, nil
}
