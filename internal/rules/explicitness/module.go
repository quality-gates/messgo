package explicitness

import (
	"go/ast"
	"go/token"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
)

// moduleName is the owner of the code that runs when a package loads: the
// package variable initializers and the init functions of a file.
const moduleName = "<module>"

// moduleOwner returns the owner that the rules report for fn. An init
// function runs when the package loads, thus its owner is the module.
func moduleOwner(fn *model.Function) *model.Function {
	if fn.Name != "init" || fn.IsMethod() || fn.Decl == nil || fn.Decl.Recv != nil {
		return fn
	}
	return &model.Function{Name: moduleName, Params: fn.Params, Results: fn.Results, Line: fn.Line,
		EndLine: fn.EndLine, Decl: fn.Decl, Body: fn.Body, File: fn.File}
}

// moduleFunction returns a function whose body evaluates the package variable
// initializers of file, or nil if file has none. The body has no position, so
// no identifier is local to it. The parser resolves each package variable
// itself, not its initializer, thus the body holds only the values.
func moduleFunction(file *model.File) *model.Function {
	if file == nil || file.Syntax == nil {
		return nil
	}
	body := &ast.BlockStmt{}
	for _, decl := range file.Syntax.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			for _, value := range spec.(*ast.ValueSpec).Values {
				body.List = append(body.List, &ast.ExprStmt{X: value})
			}
		}
	}
	if len(body.List) == 0 {
		return nil
	}
	decl := &ast.FuncDecl{Name: ast.NewIdent(moduleName), Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: body}
	return &model.Function{Name: moduleName, Decl: decl, Body: body, File: file}
}

// applyModule runs apply for the package variable initializers of the file.
func applyModule(c *rule.Context, apply func(*rule.Context, *model.Function)) {
	if fn := moduleFunction(c.File); fn != nil {
		apply(c, fn)
	}
}
