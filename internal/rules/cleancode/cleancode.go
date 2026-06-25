// Package cleancode implements PHPMD's Clean Code ruleset, adapted to Go.
// PHP-only rules (StaticAccess, ErrorControlOperator, MissingImport,
// UndefinedVariable) have no Go analog — Go's compiler already enforces the
// equivalents — and are omitted.
package cleancode

import (
	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/util"
)

func init() {
	rule.Register("PHPMD\\Rule\\CleanCode\\BooleanArgumentFlag", newBooleanArgumentFlag)
	rule.Register("PHPMD\\Rule\\CleanCode\\ElseExpression", func() rule.Rule { return &ElseExpression{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\CleanCode\\IfStatementAssignment", func() rule.Rule { return &IfStatementAssignment{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\CleanCode\\DuplicatedArrayKey", func() rule.Rule { return &DuplicatedArrayKey{Base: rule.NewBase()} })
}

// ----- BooleanArgumentFlag ------------------------------------------------
//
// Flags boolean parameters, which typically signal that a function does two
// things depending on the flag (a Single Responsibility Principle smell).

type BooleanArgumentFlag struct {
	*rule.Base
	exceptions []string
}

func newBooleanArgumentFlag() rule.Rule {
	return &BooleanArgumentFlag{Base: rule.NewBase()}
}

func (r *BooleanArgumentFlag) Configure(props rule.Properties) error {
	r.exceptions = util.SplitToList(props.String("exceptions", ""))
	return nil
}

func (r *BooleanArgumentFlag) check(c *rule.Context, fn *model.Function) {
	if fn.Receiver != "" && util.Contains(r.exceptions, fn.Receiver) {
		return
	}
	image := fn.Name
	if fn.IsMethod() {
		image = fn.Receiver + "::" + fn.Name
	}
	for _, p := range fn.Params {
		if p.Type == "bool" && p.Name != "" && p.Name != "_" {
			c.ReportFuncAt(fn, p.Line, p.Line, image, p.Name)
		}
	}
}
func (r *BooleanArgumentFlag) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- ElseExpression -----------------------------------------------------

type ElseExpression struct{ *rule.Base }

func (r *ElseExpression) check(c *rule.Context, fn *model.Function) {
	for _, line := range fn.ElseBlockLines() {
		c.ReportFuncAt(fn, line, line, fn.Name)
	}
}
func (r *ElseExpression) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- IfStatementAssignment ----------------------------------------------
//
// Flags plain assignment (=) in an if-statement initializer, e.g.
// `if x = f(); x { ... }`. Idiomatic short declarations (:=) are not flagged.

type IfStatementAssignment struct{ *rule.Base }

func (r *IfStatementAssignment) check(c *rule.Context, fn *model.Function) {
	for _, pos := range fn.IfAssignmentInitPositions() {
		c.ReportFuncAt(fn, pos.Line, pos.Line, pos.Line, pos.Column)
	}
}
func (r *IfStatementAssignment) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- DuplicatedArrayKey -------------------------------------------------
//
// Flags duplicate constant keys in a composite literal (map or array/slice
// with explicit indices).

type DuplicatedArrayKey struct{ *rule.Base }

func (r *DuplicatedArrayKey) check(c *rule.Context, fn *model.Function) {
	for _, dup := range fn.DuplicateLiteralKeys() {
		c.ReportFuncAt(fn, dup.Line, dup.Line, dup.Display, dup.FirstLine)
	}
}
func (r *DuplicatedArrayKey) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }
