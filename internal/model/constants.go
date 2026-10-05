package model

import (
	"go/ast"
	"go/token"
)

// collectConstants returns every constant declared in the file (both
// package-level and nested), the Go analog of PHPMD's ConstantDeclarator
// nodes, in source order. The blank identifier is skipped.
func collectConstants(file *ast.File, fset *token.FileSet) []Constant {
	topLevel := map[*ast.GenDecl]bool{}
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok {
			topLevel[gen] = true
		}
	}
	var out []Constant
	ast.Inspect(file, func(n ast.Node) bool {
		gen, ok := n.(*ast.GenDecl)
		if ok && gen.Tok == token.CONST {
			out = appendConstSpecs(out, gen, topLevel[gen], fset)
		}
		return true
	})
	return out
}

func appendConstSpecs(out []Constant, gen *ast.GenDecl, pkg bool, fset *token.FileSet) []Constant {
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, id := range vs.Names {
			if id.Name == "_" {
				continue
			}
			out = append(out, Constant{Name: id.Name, Line: fset.Position(id.Pos()).Line, Package: pkg})
		}
	}
	return out
}
