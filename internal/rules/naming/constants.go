package naming

import (
	"go/ast"
	"go/token"

	"github.com/quality-gates/messgo/internal/model"
)

type constDecl struct {
	name string
	line int
}

// collectConstants returns every constant declared in the file (both
// package-level and nested), the Go analog of PHPMD's ConstantDeclarator
// nodes. The blank identifier is skipped.
func collectConstants(f *model.File) []constDecl {
	var out []constDecl
	ast.Inspect(f.Syntax, func(n ast.Node) bool {
		gd, ok := n.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			return true
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, id := range vs.Names {
				if id.Name == "_" {
					continue
				}
				out = append(out, constDecl{name: id.Name, line: f.Fset.Position(id.Pos()).Line})
			}
		}
		return true
	})
	return out
}
