// Package codesize implements PHPMD's Code Size ruleset, adapted to Go.
package codesize

import (
	"github.com/quality-gates/messgo/internal/metrics"
	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
)

func init() {
	rule.Register("PHPMD\\Rule\\CyclomaticComplexity", func() rule.Rule { return &CyclomaticComplexity{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\NpathComplexity", func() rule.Rule { return &NPathComplexity{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\LongMethod", func() rule.Rule { return &LongMethod{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\LongClass", func() rule.Rule { return &LongClass{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\LongParameterList", func() rule.Rule { return &LongParameterList{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\ExcessivePublicCount", func() rule.Rule { return &ExcessivePublicCount{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\TooManyFields", func() rule.Rule { return &TooManyFields{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\TooManyMethods", func() rule.Rule { return &TooManyMethods{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\TooManyPublicMethods", func() rule.Rule { return &TooManyPublicMethods{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\WeightedMethodCount", func() rule.Rule { return &WeightedMethodCount{Base: rule.NewBase()} })
}

// ----- helpers ------------------------------------------------------------

type ignoreWhitespaceOption = bool

func funcLOC(fn *model.Function, ignoreWhitespace ignoreWhitespaceOption) int {
	if fn.Decl == nil || fn.Body == nil {
		return fn.EndLine - fn.Line + 1
	}
	if ignoreWhitespace {
		return metrics.EffectiveLinesOfCode(fn.File.Fset, fn.Decl.Pos(), fn.Decl.End(), fn.File.Src)
	}
	return fn.EndLine - fn.Line + 1
}

func classLOC(c *model.Class, ignoreWhitespace ignoreWhitespaceOption) int {
	loc := c.EndLine - c.Line + 1
	if ignoreWhitespace {
		loc = metrics.EffectiveLinesOfCode(c.File.Fset, c.Spec.Pos(), c.Spec.End(), c.File.Src)
	}
	for _, m := range c.Methods {
		loc += funcLOC(m, ignoreWhitespace)
	}
	return loc
}

// ----- CyclomaticComplexity ----------------------------------------------

type CyclomaticComplexity struct{ *rule.Base }

func (r *CyclomaticComplexity) check(c *rule.Context, fn *model.Function) {
	threshold := c.Props().Int("reportLevel", 10)
	ccn := metrics.CyclomaticComplexity(fn.Body)
	if ccn < threshold {
		return
	}
	c.ReportFunc(fn, string(fn.NodeType()), fn.Name, ccn, threshold)
}
func (r *CyclomaticComplexity) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- NPathComplexity ----------------------------------------------------

type NPathComplexity struct{ *rule.Base }

func (r *NPathComplexity) check(c *rule.Context, fn *model.Function) {
	threshold := c.Props().Int("minimum", 200)
	npath := metrics.NPathComplexity(fn.Body)
	if npath < threshold {
		return
	}
	c.ReportFunc(fn, string(fn.NodeType()), fn.Name, npath, threshold)
}
func (r *NPathComplexity) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- LongMethod (ExcessiveMethodLength) --------------------------------

type LongMethod struct{ *rule.Base }

func (r *LongMethod) check(c *rule.Context, fn *model.Function) {
	threshold := c.Props().Int("minimum", 100)
	loc := funcLOC(fn, c.Props().Bool("ignore-whitespace", false))
	if loc < threshold {
		return
	}
	c.ReportFunc(fn, string(fn.NodeType()), fn.Name, loc, threshold)
}
func (r *LongMethod) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- LongClass (ExcessiveClassLength) ----------------------------------

type LongClass struct{ *rule.Base }

func (r *LongClass) ApplyClass(c *rule.Context, class *model.Class) {
	threshold := c.Props().Int("minimum", 1000)
	loc := classLOC(class, c.Props().Bool("ignore-whitespace", false))
	if loc < threshold {
		return
	}
	c.ReportClass(class, class.Name, loc, threshold)
}

// ----- LongParameterList (ExcessiveParameterList) ------------------------

type LongParameterList struct{ *rule.Base }

func (r *LongParameterList) check(c *rule.Context, fn *model.Function) {
	threshold := c.Props().Int("minimum", 10)
	count := len(fn.Params)
	if count < threshold {
		return
	}
	c.ReportFunc(fn, string(fn.NodeType()), fn.Name, count, threshold)
}
func (r *LongParameterList) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- ExcessivePublicCount ----------------------------------------------

type ExcessivePublicCount struct{ *rule.Base }

func (r *ExcessivePublicCount) ApplyClass(c *rule.Context, class *model.Class) {
	threshold := c.Props().Int("minimum", 45)
	cis := 0
	for _, m := range class.Methods {
		if m.Exported {
			cis++
		}
	}
	for _, f := range class.Fields {
		if f.Exported {
			cis++
		}
	}
	if cis < threshold {
		return
	}
	c.ReportClass(class, string(class.NodeType()), class.Name, cis, threshold)
}

// ----- TooManyFields ------------------------------------------------------

type TooManyFields struct{ *rule.Base }

func (r *TooManyFields) ApplyClass(c *rule.Context, class *model.Class) {
	threshold := c.Props().Int("maxfields", 15)
	vars := len(class.Fields)
	if vars <= threshold {
		return
	}
	c.ReportClass(class, string(class.NodeType()), class.Name, vars, threshold)
}

// ----- TooManyMethods -----------------------------------------------------

type TooManyMethods struct{ *rule.Base }

func (r *TooManyMethods) ApplyClass(c *rule.Context, class *model.Class) {
	threshold := c.Props().Int("maxmethods", 25)
	re := rule.CompileRegex(c.Props().String("ignorepattern", "(^(set|get|is|has|with))i"))
	nom := 0
	for _, m := range class.Methods {
		if re != nil && re.MatchString(m.Name) {
			continue
		}
		nom++
	}
	if nom <= threshold {
		return
	}
	c.ReportClass(class, string(class.NodeType()), class.Name, nom, threshold)
}

// ----- TooManyPublicMethods ----------------------------------------------

type TooManyPublicMethods struct{ *rule.Base }

func (r *TooManyPublicMethods) ApplyClass(c *rule.Context, class *model.Class) {
	threshold := c.Props().Int("maxmethods", 10)
	re := rule.CompileRegex(c.Props().String("ignorepattern", "(^(set|get|is|has|with))i"))
	nom := 0
	for _, m := range class.Methods {
		if !m.Exported {
			continue
		}
		if re != nil && re.MatchString(m.Name) {
			continue
		}
		nom++
	}
	if nom <= threshold {
		return
	}
	c.ReportClass(class, string(class.NodeType()), class.Name, nom, threshold)
}

// ----- WeightedMethodCount (ExcessiveClassComplexity) --------------------

type WeightedMethodCount struct{ *rule.Base }

func (r *WeightedMethodCount) ApplyClass(c *rule.Context, class *model.Class) {
	threshold := c.Props().Int("maximum", 50)
	wmc := 0
	for _, m := range class.Methods {
		wmc += metrics.CyclomaticComplexity(m.Body)
	}
	if wmc < threshold {
		return
	}
	c.ReportClass(class, class.Name, wmc, threshold)
}
