package model

import (
	"go/ast"
	"go/token"
)

// collectConstants returns the constants for File.Constants.
func collectConstants(f *File) []Constant {
	topLevel := map[*ast.GenDecl]bool{}
	for _, decl := range f.Syntax.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok {
			topLevel[gen] = true
		}
	}
	var out []Constant
	ast.Inspect(f.Syntax, func(n ast.Node) bool {
		gen, ok := n.(*ast.GenDecl)
		if ok && gen.Tok == token.CONST {
			out = appendConstSpecs(out, gen, topLevel[gen], f.Fset)
		}
		return true
	})
	return out
}

func appendConstSpecs(out []Constant, gen *ast.GenDecl, packageLevel bool, fset *token.FileSet) []Constant {
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, id := range vs.Names {
			if id.Name == "_" {
				continue
			}
			out = append(out, Constant{Name: id.Name, Line: fset.Position(id.Pos()).Line, PackageLevel: packageLevel})
		}
	}
	return out
}
