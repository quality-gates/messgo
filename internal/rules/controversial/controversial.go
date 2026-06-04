// Package controversial implements PHPMD's Controversial ruleset, adapted to
// Go. The CamelCase rules enforce Go's MixedCaps convention (no underscores in
// identifiers). PHPMD's Superglobals rule has no Go analog and is omitted.
package controversial

import (
	"strings"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/util"
)

func init() {
	rule.Register("PHPMD\\Rule\\Controversial\\CamelCaseClassName", func() rule.Rule { return &CamelCaseClassName{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Controversial\\CamelCaseMethodName", func() rule.Rule { return &CamelCaseMethodName{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Controversial\\CamelCasePropertyName", func() rule.Rule { return &CamelCasePropertyName{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Controversial\\CamelCaseParameterName", func() rule.Rule { return &CamelCaseParameterName{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Controversial\\CamelCaseVariableName", func() rule.Rule { return &CamelCaseVariableName{Base: rule.NewBase()} })
}

// isCamelCase reports whether name follows Go's MixedCaps convention: it
// contains no underscores (the blank identifier is handled by callers).
func isCamelCase(name string) bool {
	return !strings.Contains(name, "_")
}

// ----- CamelCaseClassName -------------------------------------------------

type CamelCaseClassName struct{ *rule.Base }

func (r *CamelCaseClassName) ApplyClass(c *rule.Context, cl *model.Class) {
	if !isCamelCase(cl.Name) {
		c.Report(cl.Line, cl.EndLine, cl.Name)
	}
}
func (r *CamelCaseClassName) ApplyInterface(c *rule.Context, i *model.Interface) {
	if !isCamelCase(i.Name) {
		c.Report(i.Line, i.EndLine, i.Name)
	}
}

// ----- CamelCaseMethodName ------------------------------------------------

type CamelCaseMethodName struct{ *rule.Base }

func (r *CamelCaseMethodName) check(c *rule.Context, fn *model.Function) {
	if !isCamelCase(fn.Name) {
		c.ReportFunc(fn, fn.Name)
	}
}
func (r *CamelCaseMethodName) ApplyMethod(c *rule.Context, fn *model.Function)   { r.check(c, fn) }
func (r *CamelCaseMethodName) ApplyFunction(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- CamelCasePropertyName ----------------------------------------------

type CamelCasePropertyName struct{ *rule.Base }

func (r *CamelCasePropertyName) ApplyClass(c *rule.Context, cl *model.Class) {
	for _, f := range cl.Fields {
		if f.Name != "_" && !isCamelCase(f.Name) {
			c.Report(f.Line, f.Line, f.Name)
		}
	}
}

// ----- CamelCaseParameterName ---------------------------------------------

type CamelCaseParameterName struct{ *rule.Base }

func (r *CamelCaseParameterName) check(c *rule.Context, fn *model.Function) {
	for _, p := range fn.Params {
		if p.Name != "" && p.Name != "_" && !isCamelCase(p.Name) {
			c.Report(p.Line, p.Line, p.Name)
		}
	}
}
func (r *CamelCaseParameterName) ApplyMethod(c *rule.Context, fn *model.Function)   { r.check(c, fn) }
func (r *CamelCaseParameterName) ApplyFunction(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- CamelCaseVariableName ----------------------------------------------

type CamelCaseVariableName struct{ *rule.Base }

func (r *CamelCaseVariableName) check(c *rule.Context, fn *model.Function) {
	if fn.Body == nil {
		return
	}
	seen := map[string]bool{}
	for _, v := range util.LocalVariables(fn.Body, fn.File.Fset) {
		if v.Name == "_" || isCamelCase(v.Name) || seen[v.Name] {
			continue
		}
		seen[v.Name] = true
		c.Report(v.Line, v.Line, v.Name)
	}
}
func (r *CamelCaseVariableName) ApplyMethod(c *rule.Context, fn *model.Function)   { r.check(c, fn) }
func (r *CamelCaseVariableName) ApplyFunction(c *rule.Context, fn *model.Function) { r.check(c, fn) }
