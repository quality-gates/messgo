// Package naming implements PHPMD's Naming ruleset, adapted to Go.
package naming

import (
	"regexp"
	"strings"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/util"
)

func init() {
	rule.Register("PHPMD\\Rule\\Naming\\ShortClassName", func() rule.Rule { return &ShortClassName{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Naming\\LongClassName", func() rule.Rule { return &LongClassName{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Naming\\ShortVariable", func() rule.Rule { return &ShortVariable{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Naming\\LongVariable", func() rule.Rule { return &LongVariable{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Naming\\ShortMethodName", func() rule.Rule { return &ShortMethodName{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Naming\\BooleanGetMethodName", func() rule.Rule { return &BooleanGetMethodName{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Naming\\ConstantNamingConventions", func() rule.Rule { return &ConstantNamingConventions{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Naming\\ConstructorWithNameAsEnclosingClass", func() rule.Rule { return &ConstructorWithNameAsEnclosingClass{Base: rule.NewBase()} })
}

// ----- ShortClassName -----------------------------------------------------

type ShortClassName struct{ *rule.Base }

func (r *ShortClassName) check(c *rule.Context, name string, line, end int) {
	min := c.Props().Int("minimum", 3)
	if len(name) >= min {
		return
	}
	if util.Contains(util.SplitToList(c.Props().String("exceptions", "")), name) {
		return
	}
	c.Report(line, end, name, min)
}
func (r *ShortClassName) ApplyClass(c *rule.Context, cl *model.Class) {
	r.check(c, cl.Name, cl.Line, cl.EndLine)
}
func (r *ShortClassName) ApplyInterface(c *rule.Context, i *model.Interface) {
	r.check(c, i.Name, i.Line, i.EndLine)
}

// ----- LongClassName ------------------------------------------------------

type LongClassName struct{ *rule.Base }

func (r *LongClassName) check(c *rule.Context, name string, line, end int) {
	max := c.Props().Int("maximum", 40)
	prefixes := util.SplitToList(c.Props().String("subtract-prefixes", ""))
	suffixes := util.SplitToList(c.Props().String("subtract-suffixes", ""))
	if util.LengthWithoutPrefixesAndSuffixes(name, prefixes, suffixes) <= max {
		return
	}
	c.Report(line, end, name, max)
}
func (r *LongClassName) ApplyClass(c *rule.Context, cl *model.Class) {
	r.check(c, cl.Name, cl.Line, cl.EndLine)
}
func (r *LongClassName) ApplyInterface(c *rule.Context, i *model.Interface) {
	r.check(c, i.Name, i.Line, i.EndLine)
}

// ----- ShortVariable ------------------------------------------------------

type ShortVariable struct{ *rule.Base }

func (r *ShortVariable) checkName(c *rule.Context, name string, line int, exceptions []string) {
	min := c.Props().Int("minimum", 3)
	if len(name) >= min {
		return
	}
	if util.Contains(exceptions, name) {
		return
	}
	c.Report(line, line, name, min)
}

func (r *ShortVariable) ApplyClass(c *rule.Context, cl *model.Class) {
	ex := util.SplitToList(c.Props().String("exceptions", ""))
	for _, f := range cl.Fields {
		r.checkName(c, f.Name, f.Line, ex)
	}
}

func (r *ShortVariable) checkFunc(c *rule.Context, fn *model.Function) {
	ex := util.SplitToList(c.Props().String("exceptions", ""))
	for _, p := range fn.Params {
		if p.Name != "" {
			r.checkName(c, p.Name, p.Line, ex)
		}
	}
	if fn.Body != nil {
		for _, v := range util.LocalVariables(fn.Body, fn.File.Fset) {
			if v.IsLoop { // PHPMD allows short loop counters (for-init context)
				continue
			}
			r.checkName(c, v.Name, v.Line, ex)
		}
	}
}
func (r *ShortVariable) ApplyMethod(c *rule.Context, fn *model.Function)   { r.checkFunc(c, fn) }
func (r *ShortVariable) ApplyFunction(c *rule.Context, fn *model.Function) { r.checkFunc(c, fn) }

// ----- LongVariable -------------------------------------------------------

type LongVariable struct{ *rule.Base }

func (r *LongVariable) checkName(c *rule.Context, name string, line int) {
	max := c.Props().Int("maximum", 20)
	prefixes := util.SplitToList(c.Props().String("subtract-prefixes", ""))
	suffixes := util.SplitToList(c.Props().String("subtract-suffixes", ""))
	if util.LengthWithoutPrefixesAndSuffixes(name, prefixes, suffixes) <= max {
		return
	}
	c.Report(line, line, name, max)
}

func (r *LongVariable) ApplyClass(c *rule.Context, cl *model.Class) {
	for _, f := range cl.Fields {
		r.checkName(c, f.Name, f.Line)
	}
}
func (r *LongVariable) checkFunc(c *rule.Context, fn *model.Function) {
	for _, p := range fn.Params {
		if p.Name != "" {
			r.checkName(c, p.Name, p.Line)
		}
	}
	if fn.Body != nil {
		for _, v := range util.LocalVariables(fn.Body, fn.File.Fset) {
			r.checkName(c, v.Name, v.Line)
		}
	}
}
func (r *LongVariable) ApplyMethod(c *rule.Context, fn *model.Function)   { r.checkFunc(c, fn) }
func (r *LongVariable) ApplyFunction(c *rule.Context, fn *model.Function) { r.checkFunc(c, fn) }

// ----- ShortMethodName ----------------------------------------------------

type ShortMethodName struct{ *rule.Base }

func (r *ShortMethodName) check(c *rule.Context, fn *model.Function) {
	min := c.Props().Int("minimum", 3)
	if len(fn.Name) >= min {
		return
	}
	if util.Contains(util.SplitToList(c.Props().String("exceptions", "")), fn.Name) {
		return
	}
	c.ReportFunc(fn, fn.Receiver, fn.Name, min)
}
func (r *ShortMethodName) ApplyMethod(c *rule.Context, fn *model.Function)   { r.check(c, fn) }
func (r *ShortMethodName) ApplyFunction(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- BooleanGetMethodName -----------------------------------------------

type BooleanGetMethodName struct{ *rule.Base }

var getterRe = regexp.MustCompile(`(?i)^_?get`)

func (r *BooleanGetMethodName) check(c *rule.Context, fn *model.Function) {
	if !getterRe.MatchString(fn.Name) {
		return
	}
	if !returnsSingleBool(fn) {
		return
	}
	if c.Props().Bool("checkParameterizedMethods", false) && len(fn.Params) > 0 {
		return
	}
	c.ReportFunc(fn, fn.Name)
}

func returnsSingleBool(fn *model.Function) bool {
	if len(fn.Results) != 1 {
		return false
	}
	return fn.Results[0].Type == "bool"
}
func (r *BooleanGetMethodName) ApplyMethod(c *rule.Context, fn *model.Function)   { r.check(c, fn) }
func (r *BooleanGetMethodName) ApplyFunction(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- ConstantNamingConventions ------------------------------------------
//
// PHPMD requires class constants to be UPPERCASE. Adapted to Go, where the
// idiom is MixedCaps, this flags constant names that contain an underscore
// (i.e. ALL_CAPS or snake_case) — the Go analog of the same intent.

type ConstantNamingConventions struct{ *rule.Base }

func (r *ConstantNamingConventions) ApplyFile(c *rule.Context) {
	for _, cst := range collectConstants(c.File) {
		if strings.Contains(cst.name, "_") {
			c.Report(cst.line, cst.line, cst.name)
		}
	}
}

// ----- ConstructorWithNameAsEnclosingClass --------------------------------
//
// PHP4-style constructors named after the class. Adapted to Go: a method whose
// name equals its receiver type name (a confusing "constructor-like" method).

type ConstructorWithNameAsEnclosingClass struct{ *rule.Base }

func (r *ConstructorWithNameAsEnclosingClass) ApplyMethod(c *rule.Context, fn *model.Function) {
	if fn.Receiver != "" && fn.Name == fn.Receiver {
		c.ReportFunc(fn)
	}
}
