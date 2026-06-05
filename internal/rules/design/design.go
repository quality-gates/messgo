// Package design implements PHPMD's Design ruleset, adapted to Go. PHP-only
// rules (EvalExpression, NumberOfChildren, DepthOfInheritance) have no Go
// analog and are omitted.
package design

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/util"
)

func init() {
	rule.Register("PHPMD\\Rule\\Design\\ExitExpression", func() rule.Rule { return &ExitExpression{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\GotoStatement", func() rule.Rule { return &GotoStatement{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\CountInLoopExpression", func() rule.Rule { return &CountInLoopExpression{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\DevelopmentCodeFragment", func() rule.Rule { return &DevelopmentCodeFragment{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\EmptyCatchBlock", func() rule.Rule { return &EmptyCatchBlock{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\CouplingBetweenObjects", func() rule.Rule { return &CouplingBetweenObjects{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\GlobalVariable", func() rule.Rule { return &GlobalVariable{Base: rule.NewBase()} })
}

// ----- GlobalVariable -----------------------------------------------------
//
// Flags mutable package-level variables (top-level `var` declarations). Global
// mutable state hurts testability and is unsafe under concurrency. Only the
// file's top-level declarations are inspected, so local variables inside
// functions are never flagged; constants (`const`) are not variables and are
// likewise ignored. The blank identifier (`var _ = ...`, a common compile-time
// assertion idiom) is skipped.
type GlobalVariable struct{ *rule.Base }

func (r *GlobalVariable) ApplyFile(c *rule.Context) {
	for _, decl := range c.File.Syntax.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if name.Name == "_" {
					continue
				}
				line := c.File.Fset.Position(name.Pos()).Line
				c.Report(line, line, name.Name)
			}
		}
	}
}

// ----- ExitExpression -----------------------------------------------------
//
// Detects calls to os.Exit / syscall.Exit (the Go analog of PHP's exit/die).

type ExitExpression struct{ *rule.Base }

func (r *ExitExpression) check(c *rule.Context, fn *model.Function) {
	if fn.Body == nil {
		return
	}
	for _, call := range util.Calls(fn.Body, fn.File.Fset) {
		if call.Name == "os.Exit" || call.Name == "syscall.Exit" {
			c.ReportFuncAt(fn, call.Line, call.Line, string(fn.NodeType()), fn.Name)
			return
		}
	}
}
func (r *ExitExpression) ApplyMethod(c *rule.Context, fn *model.Function)   { r.check(c, fn) }
func (r *ExitExpression) ApplyFunction(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- GotoStatement ------------------------------------------------------

type GotoStatement struct{ *rule.Base }

func (r *GotoStatement) check(c *rule.Context, fn *model.Function) {
	if fn.Body == nil {
		return
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if b, ok := n.(*ast.BranchStmt); ok && b.Tok == token.GOTO {
			found = true
			return false
		}
		return true
	})
	if found {
		c.ReportFunc(fn, string(fn.NodeType()), fn.Name)
	}
}
func (r *GotoStatement) ApplyMethod(c *rule.Context, fn *model.Function)   { r.check(c, fn) }
func (r *GotoStatement) ApplyFunction(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- CountInLoopExpression ----------------------------------------------
//
// Detects len()/cap() calls in a loop condition, which are re-evaluated every
// iteration (the Go analog of count()/sizeof() in PHP loops).

type CountInLoopExpression struct{ *rule.Base }

var loopCountFuncs = map[string]bool{"len": true, "cap": true}

func (r *CountInLoopExpression) check(c *rule.Context, fn *model.Function) {
	if fn.Body == nil {
		return
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		fs, ok := n.(*ast.ForStmt)
		if !ok || fs.Cond == nil {
			return true
		}
		ast.Inspect(fs.Cond, func(cn ast.Node) bool {
			if ce, ok := cn.(*ast.CallExpr); ok {
				name := util.CalleeName(ce.Fun)
				if loopCountFuncs[name] {
					line := fn.File.Fset.Position(fs.Pos()).Line
					c.ReportFuncAt(fn, line, line, name, "for")
				}
			}
			return true
		})
		return true
	})
}
func (r *CountInLoopExpression) ApplyMethod(c *rule.Context, fn *model.Function)   { r.check(c, fn) }
func (r *CountInLoopExpression) ApplyFunction(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- DevelopmentCodeFragment --------------------------------------------

type DevelopmentCodeFragment struct{ *rule.Base }

func (r *DevelopmentCodeFragment) check(c *rule.Context, fn *model.Function) {
	if fn.Body == nil {
		return
	}
	unwanted := map[string]bool{}
	for _, f := range util.SplitToList(c.Props().String("unwanted-functions", "println,print")) {
		unwanted[strings.ToLower(strings.TrimSpace(f))] = true
	}
	image := fn.Name
	if fn.IsMethod() {
		image = fn.Receiver + "::" + fn.Name
	}
	for _, call := range util.Calls(fn.Body, fn.File.Fset) {
		if unwanted[strings.ToLower(call.Name)] {
			c.ReportFuncAt(fn, call.Line, call.Line, string(fn.NodeType()), image, call.Name)
		}
	}
}
func (r *DevelopmentCodeFragment) ApplyMethod(c *rule.Context, fn *model.Function)   { r.check(c, fn) }
func (r *DevelopmentCodeFragment) ApplyFunction(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- EmptyCatchBlock ----------------------------------------------------
//
// Go has no try/catch. The analog of a swallowed exception is an empty
// error-handling block: `if err != nil { }`.

type EmptyCatchBlock struct{ *rule.Base }

func (r *EmptyCatchBlock) check(c *rule.Context, fn *model.Function) {
	if fn.Body == nil {
		return
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ifs, ok := n.(*ast.IfStmt)
		if !ok || ifs.Body == nil || len(ifs.Body.List) != 0 {
			return true
		}
		if conditionChecksNil(ifs.Cond) {
			line := fn.File.Fset.Position(ifs.Pos()).Line
			c.ReportFuncAt(fn, line, line, fn.Name)
		}
		return true
	})
}

func conditionChecksNil(cond ast.Expr) bool {
	found := false
	ast.Inspect(cond, func(n ast.Node) bool {
		if be, ok := n.(*ast.BinaryExpr); ok && (be.Op == token.NEQ || be.Op == token.EQL) {
			if isNilIdent(be.X) || isNilIdent(be.Y) {
				found = true
			}
		}
		return true
	})
	return found
}

func isNilIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}
func (r *EmptyCatchBlock) ApplyMethod(c *rule.Context, fn *model.Function)   { r.check(c, fn) }
func (r *EmptyCatchBlock) ApplyFunction(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- CouplingBetweenObjects ---------------------------------------------
//
// Counts the number of distinct (non-builtin) named types a class depends on
// through its field types and method signatures — the Go analog of pdepend's
// CBO metric.

type CouplingBetweenObjects struct{ *rule.Base }

func (r *CouplingBetweenObjects) ApplyClass(c *rule.Context, class *model.Class) {
	threshold := c.Props().Int("maximum", 13)
	types := map[string]bool{}
	collect := func(t string) {
		if name := baseTypeName(t); name != "" && !builtinTypes[name] {
			types[name] = true
		}
	}
	for _, f := range class.Fields {
		collect(f.Type)
	}
	for _, m := range class.Methods {
		for _, p := range m.Params {
			collect(p.Type)
		}
		for _, res := range m.Results {
			collect(res.Type)
		}
	}
	cbo := len(types)
	if cbo >= threshold {
		c.ReportClass(class, class.Name, cbo, threshold)
	}
}

// baseTypeName strips pointer/slice/map decorations to the leading type name.
func baseTypeName(t string) string {
	t = strings.TrimLeft(t, "*[]")
	t = strings.TrimPrefix(t, "...")
	if i := strings.IndexByte(t, '['); i >= 0 {
		t = t[:i]
	}
	if i := strings.IndexByte(t, '.'); i >= 0 {
		t = t[i+1:]
	}
	return strings.TrimLeft(t, "*")
}

var builtinTypes = map[string]bool{
	"bool": true, "string": true, "int": true, "int8": true, "int16": true,
	"int32": true, "int64": true, "uint": true, "uint8": true, "uint16": true,
	"uint32": true, "uint64": true, "uintptr": true, "byte": true, "rune": true,
	"float32": true, "float64": true, "complex64": true, "complex128": true,
	"error": true, "any": true, "interface{}": true, "struct{}": true,
}
