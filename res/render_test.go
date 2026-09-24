package res

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLoadTemplatesReturnsErrors(t *testing.T) {
	if err := LoadTemplates(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected loading a missing template directory to fail")
	}
}

func TestRenderDoesNotMutateData(t *testing.T) {
	setTemplateCache(t, template.Must(template.New("page.page.gohtml").Parse("{{.name}}|{{.errors}}")))

	data := map[string]any{"name": "Ada"}
	var rendered bytes.Buffer
	if err := NewTemplate(nil, "page.page.gohtml").WithData(data).Render(&rendered); err != nil {
		t.Fatal(err)
	}

	if rendered.String() != "Ada|{}" {
		t.Fatalf("unexpected output: %q", rendered.String())
	}
	if _, ok := data["errors"]; ok {
		t.Fatal("render added errors to caller data")
	}
}

func TestRenderFuncMapIsSafeForConcurrentRenders(t *testing.T) {
	setTemplateCache(t, template.Must(template.New("page.page.gohtml").Funcs(template.FuncMap{
		"shout": strings.ToUpper,
	}).Parse("{{shout .name}}")))

	render := func() error {
		var rendered bytes.Buffer
		err := NewTemplate(nil, "page.page.gohtml").
			WithData(map[string]any{"name": "Ada"}).
			WithFuncMap(template.FuncMap{"shout": strings.ToUpper}).
			Render(&rendered)
		if rendered.String() != "ADA" {
			return &unexpectedOutput{value: rendered.String()}
		}
		return err
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := render(); err != nil {
				t.Errorf("render failed: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestLoadTemplatesFromDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.page.gohtml"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadTemplates(dir); err != nil {
		t.Fatal(err)
	}

	var rendered bytes.Buffer
	if err := NewTemplate(nil, "index.page.gohtml").Render(&rendered); err != nil {
		t.Fatal(err)
	}
	if rendered.String() != "hello" {
		t.Fatalf("unexpected output: %q", rendered.String())
	}
}

type unexpectedOutput struct{ value string }

func (e *unexpectedOutput) Error() string { return "unexpected output: " + e.value }

func setTemplateCache(t *testing.T, tmpl *template.Template) {
	t.Helper()
	templateCacheMu.Lock()
	templateCache = map[string]*template.Template{"page.page.gohtml": tmpl}
	templateCacheMu.Unlock()
}
