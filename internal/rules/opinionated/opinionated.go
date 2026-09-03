// Package opinionated implements Go-specific rules that are deliberately NOT
// part of the default `go` ruleset. They flag patterns that are sometimes a
// smell but are common enough in idiomatic Go that they would be noisy by
// default. See docs/adr/0001-go-mess-sign-backlog.md ranks 6-8.
package opinionated

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
)

func init() {
	rule.Register("messgo\\Rule\\UncheckedTypeAssertion", newUncheckedTypeAssertion)
	rule.Register("messgo\\Rule\\IdenticalBranches", newIdenticalBranches)
	rule.Register("messgo\\Rule\\StructEmbeddingDepth", newStructEmbeddingDepth)
}

// ----- UncheckedTypeAssertion ---------------------------------------------

// UncheckedTypeAssertion flags type assertions used without the comma-ok form
// (v, ok := x.(T)), which panic on a failed assertion. The comma-ok form and
// type switches (switch v := x.(type)) are safe and not flagged. Go-specific
// (no PHPMD analog); mirrors revive's and staticcheck's unchecked-type-assertion
// checks. See docs/adr/0001-go-mess-sign-backlog.md rank 6.
type UncheckedTypeAssertion struct {
	*rule.Base
}

func newUncheckedTypeAssertion() rule.Rule {
	return &UncheckedTypeAssertion{Base: rule.NewBase()}
}

func (r *UncheckedTypeAssertion) ApplyFunc(c *rule.Context, fn *model.Function) {
	if fn.Body == nil {
		return
	}
	safe := collectSafeTypeAssertions(fn.Body)
	fset := c.File.Fset
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ta, ok := n.(*ast.TypeAssertExpr)
		if !ok || ta.Type == nil || safe[ta] {
			return true
		}
		line := fset.Position(ta.Pos()).Line
		c.ReportFuncAt(fn, line, line, string(fn.NodeType()), fn.Name)
		return true
	})
}

// collectSafeTypeAssertions finds type assertions used in the comma-ok form
// (v, ok := x.(T) or var v, ok = x.(T)) so they can be excluded from the
// unchecked set.
func collectSafeTypeAssertions(body *ast.BlockStmt) map[*ast.TypeAssertExpr]bool {
	safe := map[*ast.TypeAssertExpr]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if len(n.Lhs) == 2 {
				markSafeTypeAssert(safe, n.Rhs)
			}
		case *ast.ValueSpec:
			if len(n.Names) == 2 {
				markSafeTypeAssert(safe, n.Values)
			}
		}
		return true
	})
	return safe
}

// markSafeTypeAssert marks a type assertion as safe if it is the sole value in
// a comma-ok assignment (2 LHS, 1 RHS).
func markSafeTypeAssert(safe map[*ast.TypeAssertExpr]bool, vals []ast.Expr) {
	if len(vals) != 1 {
		return
	}
	ta, ok := vals[0].(*ast.TypeAssertExpr)
	if !ok || ta.Type == nil {
		return
	}
	safe[ta] = true
}

// ----- IdenticalBranches --------------------------------------------------

// IdenticalBranches flags if/else and switch cases whose bodies are textually
// identical, which usually indicates copy-pasted logic that belongs in a single
// path. Go-specific (no PHPMD analog); mirrors revive's identical-branches and
// dupSquash. See docs/adr/0001-go-mess-sign-backlog.md rank 7.
type IdenticalBranches struct {
	*rule.Base
}

func newIdenticalBranches() rule.Rule {
	return &IdenticalBranches{Base: rule.NewBase()}
}

func (r *IdenticalBranches) ApplyFunc(c *rule.Context, fn *model.Function) {
	if fn.Body == nil {
		return
	}
	fset := c.File.Fset
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.IfStmt:
			r.checkIfElse(c, fn, n, fset)
		case *ast.SwitchStmt:
			r.checkSwitchCases(c, fn, n, fset)
		}
		return true
	})
}

func (r *IdenticalBranches) checkIfElse(c *rule.Context, fn *model.Function, n *ast.IfStmt, fset *token.FileSet) {
	elseBlock, ok := n.Else.(*ast.BlockStmt)
	if !ok {
		return
	}
	if !stmtsEqual(n.Body.List, elseBlock.List, fset) {
		return
	}
	line := fset.Position(elseBlock.Pos()).Line
	c.ReportFuncAt(fn, line, line, string(fn.NodeType()), fn.Name)
}

func (r *IdenticalBranches) checkSwitchCases(c *rule.Context, fn *model.Function, n *ast.SwitchStmt, fset *token.FileSet) {
	cases := switchCases(n.Body)
	reported := map[int]bool{}
	for i := range cases {
		if len(cases[i].Body) == 0 {
			continue
		}
		for j := i + 1; j < len(cases); j++ {
			if len(cases[j].Body) == 0 || reported[j] {
				continue
			}
			if !stmtsEqual(cases[i].Body, cases[j].Body, fset) {
				continue
			}
			reported[j] = true
			line := fset.Position(cases[j].Pos()).Line
			c.ReportFuncAt(fn, line, line, string(fn.NodeType()), fn.Name)
		}
	}
}

func switchCases(body *ast.BlockStmt) []*ast.CaseClause {
	var cases []*ast.CaseClause
	for _, s := range body.List {
		if cc, ok := s.(*ast.CaseClause); ok {
			cases = append(cases, cc)
		}
	}
	return cases
}

// stmtsEqual reports whether two statement lists produce identical formatted
// output, which is a practical test for "the same code".
func stmtsEqual(a, b []ast.Stmt, fset *token.FileSet) bool {
	return formatStmts(a, fset) == formatStmts(b, fset)
}

func formatStmts(stmts []ast.Stmt, fset *token.FileSet) string {
	var buf bytes.Buffer
	for _, s := range stmts {
		printer.Fprint(&buf, fset, s)
	}
	return buf.String()
}

// ----- StructEmbeddingDepth ------------------------------------------------

// StructEmbeddingDepth flags structs whose transitive embedding chain exceeds a
// threshold (default 3). Deep embedding chains make it hard to understand which
// promoted fields and methods a type exposes. Cross-package embeddings are
// approximate (treated as depth-0 leaves, since the AST alone cannot resolve
// them); within-package chains are followed across files. Go-specific (no PHPMD
// analog, but the conceptual cousin of PHPMD's DepthOfInheritance). See
// docs/adr/0001-go-mess-sign-backlog.md rank 8.
type StructEmbeddingDepth struct {
	*rule.Base
	*rule.ThresholdRule
}

func newStructEmbeddingDepth() rule.Rule {
	r := &StructEmbeddingDepth{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:    "maxdepth",
		Default:     3,
		Boundary:    rule.Above,
		NodeKind:    rule.ThresholdClass,
		ClassMetric: r.measure,
	})
	return r
}

func (r *StructEmbeddingDepth) measure(c *rule.Context, class *model.Class) (rule.ThresholdMeasurement, bool) {
	classes := c.File.PackageClasses
	if classes == nil {
		classes = c.File.Classes
	}
	byName := classIndex(classes)
	depth := embeddingDepth(class.Name, byName, map[string]bool{})
	return rule.ThresholdMeasurement{Value: depth, Args: []any{string(class.NodeType()), class.Name}}, true
}

func classIndex(classes []*model.Class) map[string]*model.Class {
	m := make(map[string]*model.Class, len(classes))
	for _, c := range classes {
		m[c.Name] = c
	}
	return m
}

// embeddingDepth computes the maximum transitive embedding chain length from
// name, following only same-file classes. visiting provides cycle protection.
func embeddingDepth(name string, byName map[string]*model.Class, visiting map[string]bool) int {
	if visiting[name] {
		return 0
	}
	c, ok := byName[name]
	if !ok {
		return 0
	}
	visiting[name] = true
	defer delete(visiting, name)
	maxd := 0
	for _, embed := range c.Embeds {
		d := 1 + embeddingDepth(embed, byName, visiting)
		if d > maxd {
			maxd = d
		}
	}
	return maxd
}
