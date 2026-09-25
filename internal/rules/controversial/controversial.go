// Package controversial implements PHPMD's Controversial ruleset, adapted to
// Go. The CamelCase rules enforce Go's MixedCaps convention (no underscores in
// identifiers). PHPMD's Superglobals rule has no Go analog and is omitted.
package controversial

import (
	"strings"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
)

func init() {
	rule.Register("PHPMD\\Rule\\Controversial\\CamelCaseClassName", func() rule.Rule { return &CamelCaseClassName{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Controversial\\CamelCaseMethodName", func() rule.Rule {
		return &CamelCaseMethodName{Base: rule.NewBase("allow-underscore", "allow-underscore-test")}
	})
	rule.Register("PHPMD\\Rule\\Controversial\\CamelCasePropertyName", func() rule.Rule {
		return &CamelCasePropertyName{Base: rule.NewBase("allow-underscore", "allow-underscore-test")}
	})
	rule.Register("PHPMD\\Rule\\Controversial\\CamelCaseParameterName", func() rule.Rule {
		return &CamelCaseParameterName{Base: rule.NewBase("allow-underscore")}
	})
	rule.Register("PHPMD\\Rule\\Controversial\\CamelCaseVariableName", func() rule.Rule {
		return &CamelCaseVariableName{Base: rule.NewBase("allow-underscore")}
	})
}

// isCamelCase reports whether name follows Go's MixedCaps convention: it
// contains no underscores (the blank identifier is handled by callers).
func isCamelCase(name string) bool {
	return !strings.Contains(name, "_")
}

// isCamelCaseWithOptions applies the underscore exceptions supported by the
// PHPMD ruleset while retaining the existing Go MixedCaps check.
func isCamelCaseWithOptions(name string, allowUnderscore, allowUnderscoreTest bool) bool {
	if isCamelCase(name) {
		return true
	}
	if allowUnderscoreTest && strings.HasPrefix(name, "Test") {
		return true
	}
	return allowUnderscore && strings.HasPrefix(name, "_") && len(name) > 1 && !strings.Contains(name[1:], "_")
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

type CamelCaseMethodName struct {
	*rule.Base
	allowUnderscore     bool
	allowUnderscoreTest bool
}

func (r *CamelCaseMethodName) Configure(props rule.Properties) error {
	r.allowUnderscore = props.Bool("allow-underscore", false)
	r.allowUnderscoreTest = props.Bool("allow-underscore-test", false)
	return nil
}

func (r *CamelCaseMethodName) check(c *rule.Context, fn *model.Function) {
	if !isCamelCaseWithOptions(fn.Name, r.allowUnderscore, r.allowUnderscoreTest) {
		c.ReportFunc(fn, fn.Name)
	}
}
func (r *CamelCaseMethodName) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- CamelCasePropertyName ----------------------------------------------

type CamelCasePropertyName struct {
	*rule.Base
	allowUnderscore     bool
	allowUnderscoreTest bool
}

func (r *CamelCasePropertyName) Configure(props rule.Properties) error {
	r.allowUnderscore = props.Bool("allow-underscore", false)
	r.allowUnderscoreTest = props.Bool("allow-underscore-test", false)
	return nil
}

func (r *CamelCasePropertyName) ApplyClass(c *rule.Context, cl *model.Class) {
	for _, f := range cl.Fields {
		if f.Name != "_" && !isCamelCaseWithOptions(f.Name, r.allowUnderscore, r.allowUnderscoreTest) {
			c.Report(f.Line, f.Line, f.Name)
		}
	}
}

// ----- CamelCaseParameterName ---------------------------------------------

type CamelCaseParameterName struct {
	*rule.Base
	allowUnderscore bool
}

func (r *CamelCaseParameterName) Configure(props rule.Properties) error {
	r.allowUnderscore = props.Bool("allow-underscore", false)
	return nil
}

func (r *CamelCaseParameterName) check(c *rule.Context, fn *model.Function) {
	for _, p := range fn.Params {
		if p.Name != "" && p.Name != "_" && !isCamelCaseWithOptions(p.Name, r.allowUnderscore, false) {
			c.Report(p.Line, p.Line, p.Name)
		}
	}
}
func (r *CamelCaseParameterName) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- CamelCaseVariableName ----------------------------------------------

type CamelCaseVariableName struct {
	*rule.Base
	allowUnderscore bool
}

func (r *CamelCaseVariableName) Configure(props rule.Properties) error {
	r.allowUnderscore = props.Bool("allow-underscore", false)
	return nil
}

func (r *CamelCaseVariableName) check(c *rule.Context, fn *model.Function) {
	seen := map[string]bool{}
	for _, v := range model.Locals(fn) {
		if v.Name == "_" || isCamelCaseWithOptions(v.Name, r.allowUnderscore, false) || seen[v.Name] {
			continue
		}
		seen[v.Name] = true
		c.Report(v.Line, v.Line, v.Name)
	}
}
func (r *CamelCaseVariableName) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }
