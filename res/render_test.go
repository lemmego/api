package res

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/lemmego/api/app"
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

// tokenContext supplies just the context lookup Render performs, leaving the
// rest of app.Context embedded so any other call fails loudly.
type tokenContext struct {
	app.Context
	values map[string]any
}

func (c tokenContext) Get(key string) any { return c.values[key] }

func (c tokenContext) PopSession(string) any { return nil }

// Every form that posts needs the CSRF token in a hidden _token field.
// Requiring each handler to pass it through by hand means forgetting it shows
// up as a 419 at submit time rather than as anything visible while building
// the page, so Render supplies it the same way it supplies errors.
func TestRenderSuppliesTheCSRFToken(t *testing.T) {
	setTemplateCache(t, template.Must(template.New("page.page.gohtml").Parse("{{._token}}")))

	ctx := tokenContext{values: map[string]any{"_token": "tok-abc"}}
	var rendered bytes.Buffer
	if err := NewTemplate(ctx, "page.page.gohtml").Render(&rendered); err != nil {
		t.Fatal(err)
	}

	if rendered.String() != "tok-abc" {
		t.Fatalf("expected the token from the request context, got %q", rendered.String())
	}
}

// An explicit value wins, so a caller can still override it.
func TestRenderKeepsAnExplicitCSRFToken(t *testing.T) {
	setTemplateCache(t, template.Must(template.New("page.page.gohtml").Parse("{{._token}}")))

	ctx := tokenContext{values: map[string]any{"_token": "from-context"}}
	var rendered bytes.Buffer
	err := NewTemplate(ctx, "page.page.gohtml").
		WithData(map[string]any{"_token": "explicit"}).
		Render(&rendered)
	if err != nil {
		t.Fatal(err)
	}

	if rendered.String() != "explicit" {
		t.Fatalf("explicit data was overwritten: %q", rendered.String())
	}
}

// Rendering without a context, or before the CSRF middleware has run, must
// still work rather than panic or inject an empty token.
func TestRenderWithoutACSRFToken(t *testing.T) {
	setTemplateCache(t, template.Must(template.New("page.page.gohtml").Parse("[{{._token}}]")))

	cases := map[string]app.Context{
		"nil context":        nil,
		"no token set":       tokenContext{values: map[string]any{}},
		"token not a string": tokenContext{values: map[string]any{"_token": 42}},
		"empty token":        tokenContext{values: map[string]any{"_token": ""}},
	}
	for name, ctx := range cases {
		t.Run(name, func(t *testing.T) {
			var rendered bytes.Buffer
			if err := NewTemplate(ctx, "page.page.gohtml").Render(&rendered); err != nil {
				t.Fatal(err)
			}
			if rendered.String() != "[]" {
				t.Fatalf("unexpected output: %q", rendered.String())
			}
		})
	}
}

// Render must not write the injected token back into the caller's map.
func TestRenderDoesNotMutateDataWithTheCSRFToken(t *testing.T) {
	setTemplateCache(t, template.Must(template.New("page.page.gohtml").Parse("{{._token}}")))

	data := map[string]any{"name": "Ada"}
	ctx := tokenContext{values: map[string]any{"_token": "tok-abc"}}
	if err := NewTemplate(ctx, "page.page.gohtml").WithData(data).Render(&bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	if _, ok := data["_token"]; ok {
		t.Fatal("render added _token to caller data")
	}
}

// csrf is registered at parse time as a stub so a template calling it will
// parse, and has to be rebound per render with the request's real token.
// Until it was, {{ csrf }} rendered an empty string: a form using it posted
// no _token and was rejected with 419 at submit, with nothing on the page to
// suggest why.
func TestCSRFHelperRendersTheHiddenField(t *testing.T) {
	setTemplateCache(t, template.Must(template.New("page.page.gohtml").
		Funcs(template.FuncMap{"csrf": func() template.HTML { return "" }}).
		Parse(`<form method="post">{{ csrf }}</form>`)))

	ctx := tokenContext{values: map[string]any{"_token": "tok3n-value"}}

	var out strings.Builder
	if err := NewTemplate(ctx, "page.page.gohtml").Render(&out); err != nil {
		t.Fatal(err)
	}

	page := out.String()
	if !strings.Contains(page, `name="_token"`) {
		t.Fatalf("csrf rendered no hidden field: %s", page)
	}
	if !strings.Contains(page, `value="tok3n-value"`) {
		t.Errorf("csrf rendered the wrong token: %s", page)
	}
}

// With no token on the context there is nothing to emit, and the template
// must still render rather than failing.
func TestCSRFHelperIsEmptyWithoutAToken(t *testing.T) {
	setTemplateCache(t, template.Must(template.New("page.page.gohtml").
		Funcs(template.FuncMap{"csrf": func() template.HTML { return "" }}).
		Parse(`<form method="post">{{ csrf }}</form>`)))

	var out strings.Builder
	if err := NewTemplate(nil, "page.page.gohtml").Render(&out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "_token") {
		t.Errorf("a field was rendered with no token available: %s", out.String())
	}
}
