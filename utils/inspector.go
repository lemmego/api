package utils

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"reflect"
	"strings"
)

type Pkg struct {
	name string
	dir  string
}

func NewPkg(packageName string) *Pkg {
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", packageName)
	output, err := cmd.Output()
	if err != nil {
		panic(err)
	}

	dir := strings.TrimSpace(string(output))
	return &Pkg{
		name: packageName,
		dir:  dir,
	}
}

func (p *Pkg) HasFunction(funcName string) bool {
	fset := token.NewFileSet()

	pkgs, err := parser.ParseDir(fset, p.dir, nil, 0)
	if err != nil {
		panic(err)
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok {
					if fn.Name.Name == funcName {
						return true
					}
				}
			}
		}
	}

	return false
}

func PkgName(v interface{}) string {
	t := reflect.TypeOf(v)

	// If it's a pointer, get the underlying type
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Struct:
		return t.PkgPath()
	case reflect.Interface:
		// For interfaces, we need to find a concrete type that implements it
		val := reflect.ValueOf(v)
		if val.IsNil() {
			// If the interface is nil, return its package path
			return t.PkgPath()
		}
		// Get the concrete value in the interface
		concreteVal := val.Elem()
		return concreteVal.Type().PkgPath()
	default:
		return t.PkgPath()
	}
}
