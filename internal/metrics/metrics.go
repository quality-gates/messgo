// Package metrics computes the code metrics PHPMD relies on (cyclomatic
// complexity, NPath complexity, lines of code), adapted to the Go AST.
package metrics

import (
	"go/ast"
	"go/token"
)

func ccnIncrement(n ast.Node) int {
	switch s := n.(type) {
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
		return 1
	case *ast.CaseClause:
		if len(s.List) > 0 { // skip default
			return 1
		}
	case *ast.CommClause:
		if s.Comm != nil { // skip default in select
			return 1
		}
	case *ast.BinaryExpr:
		if s.Op == token.LAND || s.Op == token.LOR {
			return 1
		}
	}
	return 0
}

// CyclomaticComplexity computes the cyclomatic complexity (CCN) of a function
// body. Mirrors pdepend's analyzer: a base of 1 plus one for each decision
// point — if, for, range, case (per case clause, excluding default), and each
// boolean operator (&&, ||). This is the same definition used by the widely
// adopted gocyclo and matches PHPMD's intent on Go code.
func CyclomaticComplexity(body *ast.BlockStmt) int {
	if body == nil {
		return 1
	}
	ccn := 1
	ast.Inspect(body, func(n ast.Node) bool {
		ccn += ccnIncrement(n)
		return true
	})
	return ccn
}

// NPathComplexity computes the NPath complexity (number of acyclic execution
// paths) of a function body using Nejmeh's algorithm, as implemented by
// pdepend's NPathComplexityAnalyzer.
func NPathComplexity(body *ast.BlockStmt) int {
	if body == nil {
		return 1
	}
	return npathStmts(body.List)
}

func npathStmts(stmts []ast.Stmt) int {
	product := 1
	for _, s := range stmts {
		product *= npathStmt(s)
	}
	return product
}

func returnStmtComplexity(n *ast.ReturnStmt) int {
	c := 0
	for _, r := range n.Results {
		c += expressionComplexity(r)
	}
	if c == 0 {
		return 1
	}
	return c
}

func npathSwitchOrSelect(s ast.Stmt) (int, bool) {
	switch n := s.(type) {
	case *ast.SwitchStmt:
		return npathSwitch(n.Body, n.Tag), true
	case *ast.TypeSwitchStmt:
		return npathSwitch(n.Body, nil), true
	case *ast.SelectStmt:
		return npathSelect(n.Body), true
	}
	return 0, false
}

func npathStmt(s ast.Stmt) int {
	if val, ok := npathSwitchOrSelect(s); ok {
		return val
	}
	switch n := s.(type) {
	case *ast.IfStmt:
		return npathIf(n)
	case *ast.ForStmt:
		return npathFor(n)
	case *ast.RangeStmt:
		// pdepend visitForeachStatement: E(iterable) + 1 + NP(body).
		return expressionComplexity(n.X) + 1 + npathStmts(n.Body.List)
	case *ast.BlockStmt:
		return npathStmts(n.List)
	case *ast.LabeledStmt:
		return npathStmt(n.Stmt)
	case *ast.ReturnStmt:
		return returnStmtComplexity(n)
	default:
		return 1
	}
}

// npathIf implements the NPath formula for if/else chains:
//
//	NP(if) = NP(else-part) + NP(if-body) + Σ expr
func npathIf(n *ast.IfStmt) int {
	expr := expressionComplexity(n.Cond)
	body := npathStmts(n.Body.List)
	var elsePart int
	switch e := n.Else.(type) {
	case nil:
		elsePart = 1 // implicit empty else
	case *ast.IfStmt:
		elsePart = npathIf(e)
	case *ast.BlockStmt:
		elsePart = npathStmts(e.List)
	default:
		elsePart = npathStmt(e)
	}
	return elsePart + body + expr
}

// npathFor follows pdepend visitForStatement: 1 + Σ E(loop expressions) +
// NP(body). Init/Cond/Post each contribute their boolean-op complexity.
func npathFor(n *ast.ForStmt) int {
	npath := 1
	npath += expressionComplexity(n.Cond)
	if a, ok := n.Init.(*ast.AssignStmt); ok {
		for _, e := range a.Rhs {
			npath += expressionComplexity(e)
		}
	}
	if a, ok := n.Post.(*ast.AssignStmt); ok {
		for _, e := range a.Rhs {
			npath += expressionComplexity(e)
		}
	}
	npath += npathStmts(n.Body.List)
	return npath
}

// npathSwitch follows pdepend visitSwitchStatement: E(tag) plus the sum over
// each case/default label of the NPath of that label's body. There is no
// special handling for a missing default — a default is just another label.
func npathSwitch(body *ast.BlockStmt, tag ast.Expr) int {
	npath := expressionComplexity(tag)
	for _, c := range body.List {
		if cc, ok := c.(*ast.CaseClause); ok {
			npath += npathStmts(cc.Body)
		}
	}
	return npath
}

// npathSelect treats each comm clause like a switch label.
func npathSelect(body *ast.BlockStmt) int {
	npath := 0
	for _, c := range body.List {
		if cc, ok := c.(*ast.CommClause); ok {
			npath += npathStmts(cc.Body)
		}
	}
	if npath == 0 {
		npath = 1
	}
	return npath
}

// expressionComplexity counts the boolean operators in an expression, which
// add execution paths (each && or || adds one), matching pdepend.
func expressionComplexity(e ast.Expr) int {
	if e == nil {
		return 0
	}
	count := 0
	ast.Inspect(e, func(n ast.Node) bool {
		if b, ok := n.(*ast.BinaryExpr); ok {
			if b.Op == token.LAND || b.Op == token.LOR {
				count++
			}
		}
		return true
	})
	return count
}

// LinesOfCode returns the number of source lines spanned by a node, inclusive
// of the start and end lines — PHPMD's `loc` metric.
func LinesOfCode(fset *token.FileSet, start, end token.Pos) int {
	return fset.Position(end).Line - fset.Position(start).Line + 1
}

// EffectiveLinesOfCode counts only lines that carry code within the span,
// skipping blank and comment-only lines — PHPMD's `eloc` metric (used by the
// ignore-whitespace option). It is approximate: comment markers inside string
// literals are not specially handled.
func EffectiveLinesOfCode(fset *token.FileSet, start, end token.Pos, src []byte) int {
	first := fset.Position(start).Line
	last := fset.Position(end).Line
	count := 0
	inBlockComment := false
	// Process from line 1 so block-comment state entering the span is correct.
	line := 1
	for _, raw := range splitLines(src) {
		if line > last {
			break
		}
		var hasCode bool
		hasCode, inBlockComment = lineHasCode(raw, inBlockComment)
		if line >= first && hasCode {
			count++
		}
		line++
	}
	return count
}

// splitLines splits source into individual lines without their terminators.
func splitLines(src []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := range len(src) {
		if src[i] == '\n' {
			lines = append(lines, src[start:i])
			start = i + 1
		}
	}
	lines = append(lines, src[start:])
	return lines
}

func isBlockCommentEnd(line []byte, i int) bool {
	return line[i] == '*' && i+1 < len(line) && line[i+1] == '/'
}

func checkComment(line []byte, i int) (isLine, isBlockStart, skipNext bool) {
	if line[i] == '/' && i+1 < len(line) {
		if line[i+1] == '/' {
			return true, false, false
		}
		if line[i+1] == '*' {
			return false, true, true
		}
	}
	return false, false, false
}

func isNonWhitespace(ch byte) bool {
	return ch != ' ' && ch != '\t' && ch != '\r'
}

// lineHasCode reports whether a line contains any code outside comments, given
// whether it begins inside a block comment, and returns the block-comment state
// at the line's end.
func lineHasCode(line []byte, inBlock bool) (hasCode, blockAfter bool) {
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inBlock {
			if isBlockCommentEnd(line, i) {
				inBlock = false
				i++
			}
			continue
		}
		isLine, isBlockStart, skipNext := checkComment(line, i)
		if isLine {
			return hasCode, false
		}
		if isBlockStart {
			inBlock = true
			if skipNext {
				i++
			}
			continue
		}
		if isNonWhitespace(ch) {
			hasCode = true
		}
	}
	return hasCode, inBlock
}
