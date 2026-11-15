package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// slog takes alternating key/value pairs, not a format string, so a printf
// verb in the message is printed literally and the arguments it meant to
// interpolate arrive as attributes instead. Nothing fails, nothing warns, and
// the line reads `Handling request for route: %s %s method=GET path=/x`.
//
// This is a static check over the package rather than a test of one call,
// because the defect is a typing habit: it recurs wherever somebody reaches
// for a logger with fmt in mind. It caught exactly one instance in this
// package and none in the other 33 modules.
func TestNoSlogMessageCarriesAPrintfVerb(t *testing.T) {
	// %% is an escaped percent and a bare % is ordinary prose ("100% of").
	// A verb is a percent followed by optional flags and a letter.
	verb := regexp.MustCompile(`%[-+# 0-9.\[\]*]*[a-zA-Z]`)

	logMethods := map[string]bool{
		"Debug": true, "Info": true, "Warn": true, "Error": true,
		"DebugContext": true, "InfoContext": true, "WarnContext": true, "ErrorContext": true,
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}

	fset := token.NewFileSet()
	checked := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !logMethods[sel.Sel.Name] {
				return true
			}

			// The message is the first argument for slog.X, and the second
			// for the Context variants, which take a ctx first.
			arg := call.Args[0]
			if strings.HasSuffix(sel.Sel.Name, "Context") && len(call.Args) > 1 {
				arg = call.Args[1]
			}

			lit, ok := arg.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			message, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			checked++

			if found := verb.FindString(strings.ReplaceAll(message, "%%", "")); found != "" {
				t.Errorf("%s: %s(%q) carries the printf verb %q; slog takes key/value pairs, so it is printed literally",
					filepath.Join(".", fset.Position(lit.Pos()).String()), sel.Sel.Name, message, found)
			}
			return true
		})
	}

	if checked == 0 {
		t.Fatal("no logging calls with a literal message were found, so this test asserted nothing")
	}
	t.Logf("checked %d logging calls with a literal message", checked)
}
