// Package codesize implements PHPMD's Code Size ruleset, adapted to Go.
package codesize

import (
	"go/ast"
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
	rule.Register("messgo\\Rule\\NestingDepth", newNestingDepth)
	rule.Register("messgo\\Rule\\ExcessiveReturnCount", newExcessiveReturnCount)
	rule.Register("messgo\\Rule\\NakedReturn", newNakedReturn)
	rule.Register("messgo\\Rule\\CognitiveComplexity", newCognitiveComplexity)
}

// ----- helpers ------------------------------------------------------------

type ignoreWhitespaceOption = bool

func funcLOC(fn *model.Function, ignoreWhitespace ignoreWhitespaceOption) int {
	if fn.Decl == nil || fn.Body == nil {
		return fn.EndLine - fn.Line + 1
	}
	if ignoreWhitespace {
		return fn.File.EffectiveLinesOfCode(fn.Decl.Pos(), fn.Decl.End())
	}
	return fn.EndLine - fn.Line + 1
}

func classLOC(c *model.Class, ignoreWhitespace ignoreWhitespaceOption) int {
	loc := c.EndLine - c.Line + 1
	if ignoreWhitespace {
		loc = c.File.EffectiveLinesOfCode(c.Spec.Pos(), c.Spec.End())
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

func interfaceNodeMeasurement(iface *model.Interface, value int) rule.ThresholdMeasurement {
	return rule.ThresholdMeasurement{Value: value, Args: []any{string(iface.NodeType()), iface.Name}}
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

// ----- CognitiveComplexity -----------------------------------------------

// CognitiveComplexity flags functions whose cognitive complexity (SonarSource
// algorithm) exceeds a threshold (default 20). Unlike cyclomatic complexity,
// cognitive complexity penalises nesting — deeply nested control flow scores
// higher than flat code with the same number of branches. Go-specific (no PHPMD
// analog); mirrors gocognit's convention. See docs/adr/0001-go-mess-sign-backlog.md.
type CognitiveComplexity struct {
	*rule.Base
	*rule.ThresholdRule
}

func newCognitiveComplexity() rule.Rule {
	r := &CognitiveComplexity{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:   "reportLevel",
		Default:    20,
		Boundary:   rule.AtOrAbove,
		NodeKind:   rule.ThresholdFunction,
		FuncMetric: r.measure,
	})
	return r
}

func (r *CognitiveComplexity) measure(_ *rule.Context, fn *model.Function) (rule.ThresholdMeasurement, bool) {
	return funcMeasurement(fn, metrics.CognitiveComplexity(fn.Decl)), true
}

// ----- NestingDepth -------------------------------------------------------

// NestingDepth flags functions whose deepest control-flow nesting exceeds a
// threshold (default 5). It is a Go-specific rule (no PHPMD analog) targeting
// the "arrow code" readability smell that cyclomatic and NPath complexity do
// not capture. See docs/adr/0001-go-mess-sign-backlog.md.
type NestingDepth struct {
	*rule.Base
	*rule.ThresholdRule
}

func newNestingDepth() rule.Rule {
	r := &NestingDepth{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:   "maxdepth",
		Default:    5,
		Boundary:   rule.Above,
		NodeKind:   rule.ThresholdFunction,
		FuncMetric: r.measure,
	})
	return r
}

func (r *NestingDepth) measure(_ *rule.Context, fn *model.Function) (rule.ThresholdMeasurement, bool) {
	return funcMeasurement(fn, metrics.NestingDepth(fn.Body)), true
}

// ----- ExcessiveReturnCount -----------------------------------------------

// ExcessiveReturnCount flags functions returning more values than a threshold
// (default 3). Go allows multiple returns, but a function returning many values
// is a smell that begs a struct result. Go-specific (no PHPMD analog); mirrors
// revive's function-result-limit. See docs/adr/0001-go-mess-sign-backlog.md.
type ExcessiveReturnCount struct {
	*rule.Base
	*rule.ThresholdRule
}

func newExcessiveReturnCount() rule.Rule {
	r := &ExcessiveReturnCount{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:   "maxresults",
		Default:    3,
		Boundary:   rule.Above,
		NodeKind:   rule.ThresholdFunction,
		FuncMetric: r.measure,
	})
	return r
}

func (r *ExcessiveReturnCount) measure(_ *rule.Context, fn *model.Function) (rule.ThresholdMeasurement, bool) {
	return funcMeasurement(fn, len(fn.Results)), true
}

// ----- NakedReturn --------------------------------------------------------

// NakedReturn flags functions with named results that use a bare return when
// the function is long or complex enough for the implicit return values to
// hurt readability. A naked return in a 3-line helper is idiomatic; in a
// 50-line function it hides what is being returned. The rule fires only when
// both conditions hold: a bare return is present AND (LOC ≥ minloc OR
// CCN ≥ minccn). Void functions are unaffected — a bare return with no named
// results is not the naked-return smell. Go-specific (no PHPMD analog);
// combines revive's bare-return with the existing LOC/CCN metrics.
// See docs/adr/0001-go-mess-sign-backlog.md.
type NakedReturn struct {
	*rule.Base
	minLOC int
	minCCN int
}

func newNakedReturn() rule.Rule {
	return &NakedReturn{Base: rule.NewBase(), minLOC: 50, minCCN: 10}
}

func (r *NakedReturn) Configure(props rule.Properties) error {
	r.minLOC = props.Int("minloc", 50)
	r.minCCN = props.Int("minccn", 10)
	return nil
}

func (r *NakedReturn) ApplyFunc(c *rule.Context, fn *model.Function) {
	if !hasNamedResults(fn) {
		return
	}
	if !hasNakedReturn(fn) {
		return
	}
	loc := funcLOC(fn, false)
	ccn := metrics.CyclomaticComplexity(fn.Body)
	if loc < r.minLOC && ccn < r.minCCN {
		return
	}
	c.ReportFunc(fn, string(fn.NodeType()), fn.Name, loc, ccn)
}

// hasNamedResults reports whether fn declares at least one named result.
func hasNamedResults(fn *model.Function) bool {
	for _, res := range fn.Results {
		if res.Name != "" {
			return true
		}
	}
	return false
}

// hasNakedReturn reports whether fn's body contains a bare return (a return
// with no result expressions).
func hasNakedReturn(fn *model.Function) bool {
	if fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if ret, ok := n.(*ast.ReturnStmt); ok && ret.Results == nil {
			found = true
			return false
		}
		return true
	})
	return found
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
		Property:          "maxmethods",
		Default:           25,
		Boundary:          rule.Above,
		NodeKind:          rule.ThresholdClass,
		ClassMetric:       r.measure,
		InterfaceMetric:   r.measureInterface,
		InterfaceProperty: "maxifacemethods",
		InterfaceDefault:  10,
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

func (r *TooManyMethods) measureInterface(_ *rule.Context, iface *model.Interface) (rule.ThresholdMeasurement, bool) {
	nom := 0
	for _, m := range iface.Methods {
		if r.ignorePattern != nil && r.ignorePattern.MatchString(m.Name) {
			continue
		}
		nom++
	}
	return interfaceNodeMeasurement(iface, nom), true
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
