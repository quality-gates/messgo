// Package unusedcode implements PHPMD's Unused Code ruleset, adapted to Go.
// "private" maps to Go's unexported (lower-cased) identifiers, usage is
// resolved within the analyzed file (the analog of PHPMD's class scope).
package unusedcode

import (
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

// selectedNames returns the set of identifiers used as a selector (x.Name) or
// struct-literal key anywhere in the file. This approximates "is this member
// referenced" within file scope.
func selectedNames(f *model.File) map[string]bool {
	return f.SelectedMemberNames()
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
	reads := fn.IdentifierReads()
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
	locals := fn.LocalVariables()
	reads := fn.IdentifierReads()
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
