// Package unusedcode implements PHPMD's Unused Code ruleset, adapted to Go.
// "private" maps to Go's unexported (lower-cased) identifiers, and usage is
// resolved against the owning type across the analyzed package.
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

// ----- UnusedPrivateField -------------------------------------------------

type UnusedPrivateField struct{ *rule.Base }

func (r *UnusedPrivateField) ApplyClass(c *rule.Context, class *model.Class) {
	for _, f := range class.Fields {
		if f.Exported || f.Name == "_" {
			continue
		}
		if !c.File.MemberSelectedForType(class.Name, f.Name) {
			c.Report(f.Line, f.Line, f.Name)
		}
	}
}

// ----- UnusedPrivateMethod ------------------------------------------------

type UnusedPrivateMethod struct{ *rule.Base }

func (r *UnusedPrivateMethod) ApplyFunc(c *rule.Context, method *model.Function) {
	if !method.IsMethod() || method.Exported {
		return
	}
	// Struct methods have an owning Class, whose name also normalizes local
	// aliases. Non-struct named types have no Class, so use the receiver name.
	typeName := method.Receiver
	if method.Class != nil {
		typeName = method.Class.Name
	}
	// A method that satisfies an interface declared in the same package is
	// used even when never selected by name (the sealed-interface idiom).
	// Only a matching signature can satisfy anything: a same-named
	// interface method with an incompatible signature leaves the concrete
	// method unused.
	if !c.File.MemberSelectedForType(typeName, method.Name) && !c.File.InterfaceMethodSatisfied(method) {
		c.ReportFunc(method, method.Name)
	}
}

// ----- UnusedFormalParameter ----------------------------------------------

type UnusedFormalParameter struct{ *rule.Base }

func (r *UnusedFormalParameter) check(c *rule.Context, fn *model.Function) {
	for _, p := range model.UnreadParameters(fn) {
		c.Report(p.Line, p.Line, p.Name)
	}
}
func (r *UnusedFormalParameter) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- UnusedLocalVariable ------------------------------------------------

type UnusedLocalVariable struct {
	*rule.Base
	exceptions         []string
	allowUnusedForeach bool
}

func newUnusedLocalVariable() rule.Rule {
	return &UnusedLocalVariable{Base: rule.NewBase("exceptions", "allow-unused-foreach-variables")}
}

func (r *UnusedLocalVariable) Configure(props rule.Properties) error {
	r.exceptions = util.SplitToList(props.String("exceptions", ""))
	r.allowUnusedForeach = props.Bool("allow-unused-foreach-variables", false)
	return nil
}

func (r *UnusedLocalVariable) check(c *rule.Context, fn *model.Function) {
	reported := map[string]bool{}
	for _, v := range model.Locals(fn) {
		if model.LocalRead(fn, v) || reported[v.Name] || util.Contains(r.exceptions, v.Name) {
			continue
		}
		if r.allowUnusedForeach && v.IsLoop {
			continue
		}
		reported[v.Name] = true
		c.Report(v.Line, v.Line, v.Name)
	}
}
func (r *UnusedLocalVariable) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }
