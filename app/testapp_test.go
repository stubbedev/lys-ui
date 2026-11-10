package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stubbedev/lys-ui/internal/sysfont"
	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// NewTestApplication is NewApplication for a test: the application is
// closed when the test ends, run or not, and the font store is built
// before it is, so its font warm-up finds nothing to scan behind what
// the test measures.
func NewTestApplication(tb testing.TB, sess *wlsession.Session, opts ...Option) *Application {
	tb.Helper()
	sysfont.Warm(tb.Context())
	a := NewApplication(sess, opts...)
	tb.Cleanup(a.Close)
	return a
}

func TestEveryTestApplicationIsBuiltToBeClosed(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	built := 0
	for _, name := range files {
		if strings.HasPrefix(name, "example") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name.Name == "NewTestApplication" {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch callee(call.Fun) {
				case "NewApplication":
					t.Errorf("%s: %s builds its application with NewApplication, which nothing closes: use NewTestApplication", fset.Position(call.Pos()), fn.Name.Name)
				case "NewTestApplication":
					built++
				}
				return true
			})
		}
	}
	if built == 0 {
		t.Fatal("found no test building an application")
	}
}

func callee(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}
