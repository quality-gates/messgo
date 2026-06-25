// Package codesize implements PHPMD's Code Size ruleset, adapted to Go.
package codesize

import (
	"regexp"

	"github.com/quality-gates/messgo/internal/metrics"
	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
)

func init() {
	rule.Register("PHPMD\\Rule\\CyclomaticComplexity", newCyclomaticComplexity)
	rule.Register("PHPMD\\Rule\\Design\\NpathComplexity", newNPathComplexity)
	rule.Register("PHPMD\\Rule\\Design\\LongMethod", newLongMethod)
	rule.Register("PHPMD\\Rule\\Design\\LongClass", newLongClass)
	rule.Register("PHPMD\\Rule\\Design\\LongParameterList", newLongParameterList)
	rule.Register("PHPMD\\Rule\\ExcessivePublicCount", newExcessivePublicCount)
	rule.Register("PHPMD\\Rule\\Design\\TooManyFields", newTooManyFields)
	rule.Register("PHPMD\\Rule\\Design\\TooManyMethods", newTooManyMethods)
	rule.Register("PHPMD\\Rule\\Design\\TooManyPublicMethods", newTooManyPublicMethods)
	rule.Register("PHPMD\\Rule\\Design\\WeightedMethodCount", newWeightedMethodCount)
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

func funcMeasurement(fn *model.Function, value int) rule.ThresholdMeasurement {
	return rule.ThresholdMeasurement{Value: value, Args: []any{string(fn.NodeType()), fn.Name}}
}

func classNodeMeasurement(class *model.Class, value int) rule.ThresholdMeasurement {
	return rule.ThresholdMeasurement{Value: value, Args: []any{string(class.NodeType()), class.Name}}
}

func classNameMeasurement(class *model.Class, value int) rule.ThresholdMeasurement {
	return rule.ThresholdMeasurement{Value: value, Args: []any{class.Name}}
}

// ----- CyclomaticComplexity ----------------------------------------------

type CyclomaticComplexity struct {
	*rule.Base
	*rule.ThresholdRule
}

func newCyclomaticComplexity() rule.Rule {
	r := &CyclomaticComplexity{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:   "reportLevel",
		Default:    10,
		Boundary:   rule.AtOrAbove,
		NodeKind:   rule.ThresholdFunction,
		FuncMetric: r.measure,
	})
	return r
}

func (r *CyclomaticComplexity) measure(_ *rule.Context, fn *model.Function) (rule.ThresholdMeasurement, bool) {
	return funcMeasurement(fn, metrics.CyclomaticComplexity(fn.Body)), true
}

// ----- NPathComplexity ----------------------------------------------------

type NPathComplexity struct {
	*rule.Base
	*rule.ThresholdRule
}

func newNPathComplexity() rule.Rule {
	r := &NPathComplexity{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:   "minimum",
		Default:    200,
		Boundary:   rule.AtOrAbove,
		NodeKind:   rule.ThresholdFunction,
		FuncMetric: r.measure,
	})
	return r
}

func (r *NPathComplexity) measure(_ *rule.Context, fn *model.Function) (rule.ThresholdMeasurement, bool) {
	return funcMeasurement(fn, metrics.NPathComplexity(fn.Body)), true
}

// ----- LongMethod (ExcessiveMethodLength) --------------------------------

type LongMethod struct {
	*rule.Base
	*rule.ThresholdRule
	ignoreWhitespace ignoreWhitespaceOption
}

func newLongMethod() rule.Rule {
	r := &LongMethod{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:   "minimum",
		Default:    100,
		Boundary:   rule.AtOrAbove,
		NodeKind:   rule.ThresholdFunction,
		FuncMetric: r.measure,
	})
	return r
}

func (r *LongMethod) Configure(props rule.Properties) error {
	if err := r.ThresholdRule.Configure(props); err != nil {
		return err
	}
	r.ignoreWhitespace = props.Bool("ignore-whitespace", false)
	return nil
}

func (r *LongMethod) measure(_ *rule.Context, fn *model.Function) (rule.ThresholdMeasurement, bool) {
	return funcMeasurement(fn, funcLOC(fn, r.ignoreWhitespace)), true
}

// ----- LongClass (ExcessiveClassLength) ----------------------------------

type LongClass struct {
	*rule.Base
	*rule.ThresholdRule
	ignoreWhitespace ignoreWhitespaceOption
}

func newLongClass() rule.Rule {
	r := &LongClass{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:    "minimum",
		Default:     1000,
		Boundary:    rule.AtOrAbove,
		NodeKind:    rule.ThresholdClass,
		ClassMetric: r.measure,
	})
	return r
}

func (r *LongClass) Configure(props rule.Properties) error {
	if err := r.ThresholdRule.Configure(props); err != nil {
		return err
	}
	r.ignoreWhitespace = props.Bool("ignore-whitespace", false)
	return nil
}

func (r *LongClass) measure(_ *rule.Context, class *model.Class) (rule.ThresholdMeasurement, bool) {
	return classNameMeasurement(class, classLOC(class, r.ignoreWhitespace)), true
}

// ----- LongParameterList (ExcessiveParameterList) ------------------------

type LongParameterList struct {
	*rule.Base
	*rule.ThresholdRule
}

func newLongParameterList() rule.Rule {
	r := &LongParameterList{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:   "minimum",
		Default:    10,
		Boundary:   rule.AtOrAbove,
		NodeKind:   rule.ThresholdFunction,
		FuncMetric: r.measure,
	})
	return r
}

func (r *LongParameterList) measure(_ *rule.Context, fn *model.Function) (rule.ThresholdMeasurement, bool) {
	return funcMeasurement(fn, len(fn.Params)), true
}

// ----- ExcessivePublicCount ----------------------------------------------

type ExcessivePublicCount struct {
	*rule.Base
	*rule.ThresholdRule
}

func newExcessivePublicCount() rule.Rule {
	r := &ExcessivePublicCount{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:    "minimum",
		Default:     45,
		Boundary:    rule.AtOrAbove,
		NodeKind:    rule.ThresholdClass,
		ClassMetric: r.measure,
	})
	return r
}

func (r *ExcessivePublicCount) measure(_ *rule.Context, class *model.Class) (rule.ThresholdMeasurement, bool) {
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
	return classNodeMeasurement(class, cis), true
}

// ----- TooManyFields ------------------------------------------------------

type TooManyFields struct {
	*rule.Base
	*rule.ThresholdRule
}

func newTooManyFields() rule.Rule {
	r := &TooManyFields{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:    "maxfields",
		Default:     15,
		Boundary:    rule.Above,
		NodeKind:    rule.ThresholdClass,
		ClassMetric: r.measure,
	})
	return r
}

func (r *TooManyFields) measure(_ *rule.Context, class *model.Class) (rule.ThresholdMeasurement, bool) {
	return classNodeMeasurement(class, len(class.Fields)), true
}

// ----- TooManyMethods -----------------------------------------------------

type TooManyMethods struct {
	*rule.Base
	*rule.ThresholdRule
	ignorePattern *regexp.Regexp
}

func newTooManyMethods() rule.Rule {
	r := &TooManyMethods{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:    "maxmethods",
		Default:     25,
		Boundary:    rule.Above,
		NodeKind:    rule.ThresholdClass,
		ClassMetric: r.measure,
	})
	return r
}

func (r *TooManyMethods) Configure(props rule.Properties) error {
	if err := r.ThresholdRule.Configure(props); err != nil {
		return err
	}
	r.ignorePattern = rule.CompileRegex(props.String("ignorepattern", "(^(set|get|is|has|with))i"))
	return nil
}

func (r *TooManyMethods) measure(_ *rule.Context, class *model.Class) (rule.ThresholdMeasurement, bool) {
	nom := 0
	for _, m := range class.Methods {
		if r.ignorePattern != nil && r.ignorePattern.MatchString(m.Name) {
			continue
		}
		nom++
	}
	return classNodeMeasurement(class, nom), true
}

// ----- TooManyPublicMethods ----------------------------------------------

type TooManyPublicMethods struct {
	*rule.Base
	*rule.ThresholdRule
	ignorePattern *regexp.Regexp
}

func newTooManyPublicMethods() rule.Rule {
	r := &TooManyPublicMethods{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:    "maxmethods",
		Default:     10,
		Boundary:    rule.Above,
		NodeKind:    rule.ThresholdClass,
		ClassMetric: r.measure,
	})
	return r
}

func (r *TooManyPublicMethods) Configure(props rule.Properties) error {
	if err := r.ThresholdRule.Configure(props); err != nil {
		return err
	}
	r.ignorePattern = rule.CompileRegex(props.String("ignorepattern", "(^(set|get|is|has|with))i"))
	return nil
}

func (r *TooManyPublicMethods) measure(_ *rule.Context, class *model.Class) (rule.ThresholdMeasurement, bool) {
	nom := 0
	for _, m := range class.Methods {
		if !m.Exported {
			continue
		}
		if r.ignorePattern != nil && r.ignorePattern.MatchString(m.Name) {
			continue
		}
		nom++
	}
	return classNodeMeasurement(class, nom), true
}

// ----- WeightedMethodCount (ExcessiveClassComplexity) --------------------

type WeightedMethodCount struct {
	*rule.Base
	*rule.ThresholdRule
}

func newWeightedMethodCount() rule.Rule {
	r := &WeightedMethodCount{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:    "maximum",
		Default:     50,
		Boundary:    rule.AtOrAbove,
		NodeKind:    rule.ThresholdClass,
		ClassMetric: r.measure,
	})
	return r
}

func (r *WeightedMethodCount) measure(_ *rule.Context, class *model.Class) (rule.ThresholdMeasurement, bool) {
	wmc := 0
	for _, m := range class.Methods {
		wmc += metrics.CyclomaticComplexity(m.Body)
	}
	return classNameMeasurement(class, wmc), true
}
