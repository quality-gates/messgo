package rule

import "github.com/quality-gates/messgo/internal/model"

// RuleSet is a named collection of rules (PHPMD\RuleSet).
type RuleSet struct {
	Name        string
	Description string
	Rules       []Rule
}

// Analyze runs all rules in all rule sets against a single parsed file and
// returns the violations found. Dispatch mirrors PHPMD: each rule is invoked
// for every artifact of the kind(s) it is aware of.
func Analyze(file *model.File, sets []*RuleSet) []*Violation {
	var violations []*Violation
	for _, set := range sets {
		for _, r := range set.Rules {
			ctx := &Context{
				File:       file,
				violations: &violations,
				rule:       r,
			}
			if br, ok := r.(BaseRef); ok {
				ctx.props = br.base().RuleProps
			}
			applyRule(ctx, r, file)
		}
	}
	return violations
}

func applyRule(ctx *Context, r Rule, file *model.File) {
	if fr, ok := r.(FileRule); ok {
		fr.ApplyFile(ctx)
	}
	if cr, ok := r.(ClassRule); ok {
		for _, c := range file.Classes {
			cr.ApplyClass(ctx, c)
		}
	}
	if ir, ok := r.(InterfaceRule); ok {
		for _, i := range file.Interfaces {
			ir.ApplyInterface(ctx, i)
		}
	}
	if fr, ok := r.(FuncRule); ok {
		applyFuncRule(ctx, fr, file)
	}
}

// applyFuncRule invokes a function-aware rule once for every function-level
// artifact: free functions, methods (both in AllFuncs), and interface methods
// (which PHPMD also models as methods). Method-only rules guard on
// fn.IsMethod() in their own body.
func applyFuncRule(ctx *Context, fr FuncRule, file *model.File) {
	for _, fn := range file.AllFuncs {
		fr.ApplyFunc(ctx, fn)
	}
	for _, iface := range file.Interfaces {
		for _, fn := range iface.Methods {
			fr.ApplyFunc(ctx, fn)
		}
	}
}
