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
	rule.Register("PHPMD\\Rule\\Naming\\ShortClassName", newShortClassName)
	rule.Register("PHPMD\\Rule\\Naming\\LongClassName", newLongClassName)
	rule.Register("PHPMD\\Rule\\Naming\\ShortVariable", newShortVariable)
	rule.Register("PHPMD\\Rule\\Naming\\LongVariable", newLongVariable)
	rule.Register("PHPMD\\Rule\\Naming\\ShortMethodName", newShortMethodName)
	rule.Register("PHPMD\\Rule\\Naming\\BooleanGetMethodName", newBooleanGetMethodName)
	rule.Register("PHPMD\\Rule\\Naming\\ConstantNamingConventions", func() rule.Rule { return &ConstantNamingConventions{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Naming\\ConstructorWithNameAsEnclosingClass", func() rule.Rule { return &ConstructorWithNameAsEnclosingClass{Base: rule.NewBase()} })
}

// ----- ShortClassName -----------------------------------------------------

type ShortClassName struct {
	*rule.Base
	minimum    int
	exceptions []string
}

func newShortClassName() rule.Rule {
	return &ShortClassName{Base: rule.NewBase()}
}

func (r *ShortClassName) Configure(props rule.Properties) error {
	r.minimum = props.Int("minimum", 3)
	r.exceptions = util.SplitToList(props.String("exceptions", ""))
	return nil
}

func (r *ShortClassName) check(c *rule.Context, name string, line, end int) {
	if len(name) >= r.minimum {
		return
	}
	if util.Contains(r.exceptions, name) {
		return
	}
	c.Report(line, end, name, r.minimum)
}
func (r *ShortClassName) ApplyClass(c *rule.Context, cl *model.Class) {
	r.check(c, cl.Name, cl.Line, cl.EndLine)
}
func (r *ShortClassName) ApplyInterface(c *rule.Context, i *model.Interface) {
	r.check(c, i.Name, i.Line, i.EndLine)
}

// ----- LongClassName ------------------------------------------------------

type LongClassName struct {
	*rule.Base
	maximum  int
	prefixes []string
	suffixes []string
}

func newLongClassName() rule.Rule {
	return &LongClassName{Base: rule.NewBase()}
}

func (r *LongClassName) Configure(props rule.Properties) error {
	r.maximum = props.Int("maximum", 40)
	r.prefixes = util.SplitToList(props.String("subtract-prefixes", ""))
	r.suffixes = util.SplitToList(props.String("subtract-suffixes", ""))
	return nil
}

func (r *LongClassName) check(c *rule.Context, name string, line, end int) {
	if util.LengthWithoutPrefixesAndSuffixes(name, r.prefixes, r.suffixes) <= r.maximum {
		return
	}
	c.Report(line, end, name, r.maximum)
}
func (r *LongClassName) ApplyClass(c *rule.Context, cl *model.Class) {
	r.check(c, cl.Name, cl.Line, cl.EndLine)
}
func (r *LongClassName) ApplyInterface(c *rule.Context, i *model.Interface) {
	r.check(c, i.Name, i.Line, i.EndLine)
}

// ----- ShortVariable ------------------------------------------------------

type ShortVariable struct {
	*rule.Base
	minimum    int
	exceptions []string
}

func newShortVariable() rule.Rule {
	return &ShortVariable{Base: rule.NewBase()}
}

func (r *ShortVariable) Configure(props rule.Properties) error {
	r.minimum = props.Int("minimum", 3)
	r.exceptions = util.SplitToList(props.String("exceptions", ""))
	return nil
}

func (r *ShortVariable) checkName(c *rule.Context, name string, line int) {
	if len(name) >= r.minimum {
		return
	}
	if util.Contains(r.exceptions, name) {
		return
	}
	c.Report(line, line, name, r.minimum)
}

func (r *ShortVariable) ApplyClass(c *rule.Context, cl *model.Class) {
	for _, f := range cl.Fields {
		r.checkName(c, f.Name, f.Line)
	}
}

func (r *ShortVariable) checkFunc(c *rule.Context, fn *model.Function) {
	for _, p := range fn.Params {
		if p.Name != "" {
			r.checkName(c, p.Name, p.Line)
		}
	}
	if fn.Body != nil {
		for _, v := range util.LocalVariables(fn.Body, fn.File.Fset) {
			if v.IsLoop { // PHPMD allows short loop counters (for-init context)
				continue
			}
			r.checkName(c, v.Name, v.Line)
		}
	}
}
func (r *ShortVariable) ApplyFunc(c *rule.Context, fn *model.Function) { r.checkFunc(c, fn) }

// ----- LongVariable -------------------------------------------------------

type LongVariable struct {
	*rule.Base
	maximum  int
	prefixes []string
	suffixes []string
}

func newLongVariable() rule.Rule {
	return &LongVariable{Base: rule.NewBase()}
}

func (r *LongVariable) Configure(props rule.Properties) error {
	r.maximum = props.Int("maximum", 20)
	r.prefixes = util.SplitToList(props.String("subtract-prefixes", ""))
	r.suffixes = util.SplitToList(props.String("subtract-suffixes", ""))
	return nil
}

func (r *LongVariable) checkName(c *rule.Context, name string, line int) {
	if util.LengthWithoutPrefixesAndSuffixes(name, r.prefixes, r.suffixes) <= r.maximum {
		return
	}
	c.Report(line, line, name, r.maximum)
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
func (r *LongVariable) ApplyFunc(c *rule.Context, fn *model.Function) { r.checkFunc(c, fn) }

// ----- ShortMethodName ----------------------------------------------------

type ShortMethodName struct {
	*rule.Base
	minimum    int
	exceptions []string
}

func newShortMethodName() rule.Rule {
	return &ShortMethodName{Base: rule.NewBase()}
}

func (r *ShortMethodName) Configure(props rule.Properties) error {
	r.minimum = props.Int("minimum", 3)
	r.exceptions = util.SplitToList(props.String("exceptions", ""))
	return nil
}

func (r *ShortMethodName) check(c *rule.Context, fn *model.Function) {
	if len(fn.Name) >= r.minimum {
		return
	}
	if util.Contains(r.exceptions, fn.Name) {
		return
	}
	c.ReportFunc(fn, fn.Receiver, fn.Name, r.minimum)
}
func (r *ShortMethodName) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- BooleanGetMethodName -----------------------------------------------

type BooleanGetMethodName struct {
	*rule.Base
	checkParameterizedMethods bool
}

func newBooleanGetMethodName() rule.Rule {
	return &BooleanGetMethodName{Base: rule.NewBase()}
}

func (r *BooleanGetMethodName) Configure(props rule.Properties) error {
	r.checkParameterizedMethods = props.Bool("checkParameterizedMethods", false)
	return nil
}

var getterRe = regexp.MustCompile(`(?i)^_?get`)

func (r *BooleanGetMethodName) check(c *rule.Context, fn *model.Function) {
	if !getterRe.MatchString(fn.Name) {
		return
	}
	if !returnsSingleBool(fn) {
		return
	}
	if r.checkParameterizedMethods && len(fn.Params) > 0 {
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
func (r *BooleanGetMethodName) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

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

func (r *ConstructorWithNameAsEnclosingClass) ApplyFunc(c *rule.Context, fn *model.Function) {
	if !fn.IsMethod() {
		return
	}
	if fn.Name == fn.Receiver {
		c.ReportFunc(fn)
	}
}
