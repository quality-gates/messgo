// Package unusedcode implements PHPMD's Unused Code ruleset, adapted to Go.
// "private" maps to Go's unexported (lower-cased) identifiers, and usage is
// resolved within the analyzed file (the analog of PHPMD's class scope).
package unusedcode

import (
	"go/ast"
	"go/token"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/util"
)

func init() {
	rule.Register("PHPMD\\Rule\\UnusedPrivateField", func() rule.Rule { return &UnusedPrivateField{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\UnusedLocalVariable", newUnusedLocalVariable)
	rule.Register("PHPMD\\Rule\\UnusedPrivateMethod", func() rule.Rule { return &UnusedPrivateMethod{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\UnusedFormalParameter", func() rule.Rule { return &UnusedFormalParameter{Base: rule.NewBase()} })
}

// selectedNames returns the set of all identifiers used as a selector (x.Name)
// or as a struct-literal key anywhere in the file. This approximates "is this
// member referenced" within file scope.
func selectedNames(f *model.File) map[string]bool {
	set := map[string]bool{}
	ast.Inspect(f.Syntax, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.SelectorExpr:
			set[e.Sel.Name] = true
		case *ast.CompositeLit:
			for _, elt := range e.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if id, ok := kv.Key.(*ast.Ident); ok {
						set[id.Name] = true
					}
				}
			}
		}
		return true
	})
	return set
}

// ----- UnusedPrivateField -------------------------------------------------

type UnusedPrivateField struct{ *rule.Base }

func (r *UnusedPrivateField) ApplyClass(c *rule.Context, class *model.Class) {
	used := selectedNames(c.File)
	for _, f := range class.Fields {
		if f.Exported || f.Name == "_" {
			continue
		}
		if !used[f.Name] {
			c.Report(f.Line, f.Line, f.Name)
		}
	}
}

// ----- UnusedPrivateMethod ------------------------------------------------

type UnusedPrivateMethod struct{ *rule.Base }

func (r *UnusedPrivateMethod) ApplyClass(c *rule.Context, class *model.Class) {
	used := selectedNames(c.File)
	for _, m := range class.Methods {
		if m.Exported {
			continue
		}
		if !used[m.Name] {
			c.ReportFunc(m, m.Name)
		}
	}
}

// ----- UnusedFormalParameter ----------------------------------------------

type UnusedFormalParameter struct{ *rule.Base }

func (r *UnusedFormalParameter) check(c *rule.Context, fn *model.Function) {
	if fn.Body == nil {
		return
	}
	reads := identReads(fn.Body)
	for _, p := range fn.Params {
		if p.Name == "" || p.Name == "_" {
			continue
		}
		if !reads[p.Name] {
			c.Report(p.Line, p.Line, p.Name)
		}
	}
}
func (r *UnusedFormalParameter) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- UnusedLocalVariable ------------------------------------------------

type UnusedLocalVariable struct {
	*rule.Base
	exceptions []string
}

func newUnusedLocalVariable() rule.Rule {
	return &UnusedLocalVariable{Base: rule.NewBase()}
}

func (r *UnusedLocalVariable) Configure(props rule.Properties) error {
	r.exceptions = util.SplitToList(props.String("exceptions", ""))
	return nil
}

func (r *UnusedLocalVariable) check(c *rule.Context, fn *model.Function) {
	if fn.Body == nil {
		return
	}
	locals := util.LocalVariables(fn.Body, fn.File.Fset)
	reads := identReads(fn.Body)
	reported := map[string]bool{}
	for _, v := range locals {
		if reads[v.Name] || reported[v.Name] || util.Contains(r.exceptions, v.Name) {
			continue
		}
		reported[v.Name] = true
		c.Report(v.Line, v.Line, v.Name)
	}
}
func (r *UnusedLocalVariable) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// identReads returns the set of identifier names that are *read* somewhere in
// the body — i.e. referenced outside of the left-hand side of an assignment or
// declaration. A name written but never read is therefore not in the set,
// matching PHPMD's "only appears as assignment target" definition of unused.
func collectAssignWrites(s *ast.AssignStmt, writeIdents map[*ast.Ident]bool) {
	if s.Tok == token.ASSIGN || s.Tok == token.DEFINE {
		for _, lhs := range s.Lhs {
			if id, ok := lhs.(*ast.Ident); ok {
				writeIdents[id] = true
			}
		}
	}
}

func collectRangeWrites(s *ast.RangeStmt, writeIdents map[*ast.Ident]bool) {
	if s.Tok == token.ASSIGN || s.Tok == token.DEFINE {
		if id, ok := s.Key.(*ast.Ident); ok {
			writeIdents[id] = true
		}
		if id, ok := s.Value.(*ast.Ident); ok {
			writeIdents[id] = true
		}
	}
}

func collectWriteIdents(body *ast.BlockStmt) map[*ast.Ident]bool {
	writeIdents := map[*ast.Ident]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			collectAssignWrites(s, writeIdents)
		case *ast.ValueSpec:
			for _, id := range s.Names {
				writeIdents[id] = true
			}
		case *ast.RangeStmt:
			collectRangeWrites(s, writeIdents)
		}
		return true
	})
	return writeIdents
}

// identReads returns the set of identifier names that are *read* somewhere in
// the body — i.e. referenced outside of the left-hand side of an assignment or
// declaration. A name written but never read is therefore not in the set,
// matching PHPMD's "only appears as assignment target" definition of unused.
func identReads(body *ast.BlockStmt) map[string]bool {
	reads := map[string]bool{}
	writeIdents := collectWriteIdents(body)
	ast.Inspect(body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id.Name == "_" {
			return true
		}
		if writeIdents[id] {
			return true
		}
		reads[id.Name] = true
		return true
	})
	return reads
}
