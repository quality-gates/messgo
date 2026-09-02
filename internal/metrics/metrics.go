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

// NestingDepth computes the maximum control-flow nesting depth within a
// function body — the deepest stack of nested if/for/range/switch/type-switch/
// select statements. This is the "arrow code" smell: deeply nested control
// flow is hard to read and a candidate for early returns or guard clauses. It
// is independent of cyclomatic and NPath complexity, which do not penalise
// nesting. Mirrors the intent of nestif and revive's max-control-nesting.
//
// An else-if chain does not add depth (the chained if sits at the same level as
// its parent); an else block does. Case clauses sit inside their switch/select
// and do not add a level of their own.
func NestingDepth(body *ast.BlockStmt) int {
	if body == nil {
		return 0
	}
	return maxStmtNesting(body.List, 0)
}

func maxStmtNesting(stmts []ast.Stmt, depth int) int {
	m := depth
	for _, s := range stmts {
		if d := nestingDepthStmt(s, depth); d > m {
			m = d
		}
	}
	return m
}

// nestingDepthStmt returns the max nesting depth reachable from s, where depth
// is the nesting level of the block containing s.
func nestingDepthStmt(s ast.Stmt, depth int) int {
	switch n := s.(type) {
	case *ast.IfStmt:
		return nestingDepthIf(n, depth)
	case *ast.ForStmt:
		return max(depth+1, maxStmtNesting(n.Body.List, depth+1))
	case *ast.RangeStmt:
		return max(depth+1, maxStmtNesting(n.Body.List, depth+1))
	case *ast.SwitchStmt:
		return max(depth+1, caseNesting(n.Body.List, depth+1))
	case *ast.TypeSwitchStmt:
		return max(depth+1, caseNesting(n.Body.List, depth+1))
	case *ast.SelectStmt:
		return max(depth+1, commNesting(n.Body.List, depth+1))
	}
	return depth
}

// nestingDepthIf handles the if/else branching rules: the if body and an else
// block sit at depth+1; an else-if chain stays at the parent's depth.
func nestingDepthIf(n *ast.IfStmt, depth int) int {
	m := max(depth+1, maxStmtNesting(n.Body.List, depth+1))
	if n.Else == nil {
		return m
	}
	if blk, ok := n.Else.(*ast.BlockStmt); ok {
		return max(m, maxStmtNesting(blk.List, depth+1))
	}
	// else-if chain: the chained if sits at the same level as its parent.
	return max(m, nestingDepthStmt(n.Else, depth))
}

// caseNesting measures nesting within a switch body, whose direct children are
// CaseClauses. The case body sits at the switch's level (depth), not deeper.
func caseNesting(clauses []ast.Stmt, depth int) int {
	m := depth
	for _, c := range clauses {
		cc, ok := c.(*ast.CaseClause)
		if !ok {
			continue
		}
		if d := maxStmtNesting(cc.Body, depth); d > m {
			m = d
		}
	}
	return m
}

// commNesting measures nesting within a select body, whose direct children are
// CommClauses.
func commNesting(clauses []ast.Stmt, depth int) int {
	m := depth
	for _, c := range clauses {
		cc, ok := c.(*ast.CommClause)
		if !ok {
			continue
		}
		if d := maxStmtNesting(cc.Body, depth); d > m {
			m = d
		}
	}
	return m
}

// CognitiveComplexity computes the cognitive complexity of a function using the
// SonarSource algorithm, as implemented by gocognit (github.com/uudashr/gocognit).
// Unlike cyclomatic complexity, cognitive complexity penalises nesting: a
// control-flow structure nested inside another scores nesting+1, not just 1.
// An else-if chain does not add nesting (the chained if sits at the same level
// as its parent); an else block adds +1. Boolean && and || sequences add 1 per
// operator change. Direct recursion adds +1. Labeled break/continue adds +1.
func CognitiveComplexity(fn *ast.FuncDecl) int {
	if fn == nil {
		return 0
	}
	v := &cognitiveVisitor{name: fn.Name}
	ast.Walk(v, fn)
	return v.complexity
}

type cognitiveVisitor struct {
	name            *ast.Ident
	complexity      int
	nesting         int
	elseNodes       map[ast.Node]bool
	calculatedExprs map[ast.Expr]bool
}

func (v *cognitiveVisitor) inc()        { v.complexity++ }
func (v *cognitiveVisitor) nestInc()    { v.complexity += v.nesting + 1 }
func (v *cognitiveVisitor) incNesting() { v.nesting++ }
func (v *cognitiveVisitor) decNesting() { v.nesting-- }

func (v *cognitiveVisitor) markElse(n ast.Node) {
	if v.elseNodes == nil {
		v.elseNodes = make(map[ast.Node]bool)
	}
	v.elseNodes[n] = true
}

func (v *cognitiveVisitor) isElse(n ast.Node) bool {
	return v.elseNodes != nil && v.elseNodes[n]
}

func (v *cognitiveVisitor) markCalc(e ast.Expr) {
	if v.calculatedExprs == nil {
		v.calculatedExprs = make(map[ast.Expr]bool)
	}
	v.calculatedExprs[e] = true
}

func (v *cognitiveVisitor) isCalc(e ast.Expr) bool {
	return v.calculatedExprs != nil && v.calculatedExprs[e]
}

func (v *cognitiveVisitor) walkIfSet(n ast.Node) {
	if n != nil {
		ast.Walk(v, n)
	}
}

// Visit implements ast.Visitor. It handles nesting-increasing nodes (if, switch,
// for, range, select, func-lit) and delegates non-nesting nodes (branch, binary,
// call) to visitNonNesting.
func (v *cognitiveVisitor) Visit(n ast.Node) ast.Visitor {
	switch n := n.(type) {
	case *ast.IfStmt:
		return v.visitIf(n)
	case *ast.SwitchStmt:
		v.visitSwitch(n.Init, n.Tag, n.Body)
		return nil
	case *ast.TypeSwitchStmt:
		v.visitSwitch(n.Init, n.Assign, n.Body)
		return nil
	case *ast.SelectStmt:
		v.nestInc()
		v.incNesting()
		ast.Walk(v, n.Body)
		v.decNesting()
		return nil
	case *ast.ForStmt:
		v.nestInc()
		v.walkIfSet(n.Init)
		v.walkIfSet(n.Cond)
		v.walkIfSet(n.Post)
		v.incNesting()
		ast.Walk(v, n.Body)
		v.decNesting()
		return nil
	case *ast.RangeStmt:
		v.nestInc()
		v.walkIfSet(n.Key)
		v.walkIfSet(n.Value)
		ast.Walk(v, n.X)
		v.incNesting()
		ast.Walk(v, n.Body)
		v.decNesting()
		return nil
	case *ast.FuncLit:
		ast.Walk(v, n.Type)
		v.incNesting()
		ast.Walk(v, n.Body)
		v.decNesting()
		return nil
	}
	return v.visitNonNesting(n)
}

func (v *cognitiveVisitor) visitNonNesting(n ast.Node) ast.Visitor {
	switch n := n.(type) {
	case *ast.BranchStmt:
		if n.Label != nil {
			v.inc()
		}
		return v
	case *ast.BinaryExpr:
		v.visitBinary(n)
		return v
	case *ast.CallExpr:
		v.visitCall(n)
		return v
	}
	return v
}

func (v *cognitiveVisitor) visitIf(n *ast.IfStmt) ast.Visitor {
	if v.isElse(n) {
		v.inc()
	} else {
		v.nestInc()
	}
	v.walkIfSet(n.Init)
	ast.Walk(v, n.Cond)
	v.incNesting()
	ast.Walk(v, n.Body)
	v.decNesting()
	if blk, ok := n.Else.(*ast.BlockStmt); ok {
		v.inc() // +1 for the else keyword
		ast.Walk(v, blk)
	} else if _, ok := n.Else.(*ast.IfStmt); ok {
		v.markElse(n.Else)
		ast.Walk(v, n.Else)
	}
	return nil
}

func (v *cognitiveVisitor) visitSwitch(init ast.Stmt, extra ast.Node, body *ast.BlockStmt) {
	v.nestInc()
	v.walkIfSet(init)
	if extra != nil {
		ast.Walk(v, extra)
	}
	v.incNesting()
	ast.Walk(v, body)
	v.decNesting()
}

func (v *cognitiveVisitor) visitBinary(n *ast.BinaryExpr) {
	if !(n.Op == token.LAND || n.Op == token.LOR) {
		return
	}
	if v.isCalc(n) {
		return
	}
	ops := v.collectBinaryOps(n)
	var lastOp token.Token
	for _, op := range ops {
		if lastOp != op {
			v.inc()
			lastOp = op
		}
	}
}

func (v *cognitiveVisitor) collectBinaryOps(e ast.Expr) []token.Token {
	v.markCalc(e)
	if be, ok := e.(*ast.BinaryExpr); ok {
		return mergeBinaryOps(v.collectBinaryOps(be.X), be.Op, v.collectBinaryOps(be.Y))
	}
	return nil
}

func mergeBinaryOps(x []token.Token, op token.Token, y []token.Token) []token.Token {
	var out []token.Token
	out = append(out, x...)
	if op == token.LAND || op == token.LOR {
		out = append(out, op)
	}
	out = append(out, y...)
	return out
}

func (v *cognitiveVisitor) visitCall(n *ast.CallExpr) {
	if v.name == nil {
		return
	}
	id, ok := n.Fun.(*ast.Ident)
	if !ok {
		return
	}
	if id.Obj == v.name.Obj && id.Name == v.name.Name {
		v.inc() // direct recursion
	}
}

func npathStmts(stmts []ast.Stmt) int {
	product := 1
	for _, s := range stmts {
		product = npathMul(product, npathStmt(s))
	}
	return product
}

func returnStmtComplexity(n *ast.ReturnStmt) int {
	c := 0
	for _, r := range n.Results {
		c = npathAdd(c, expressionComplexity(r))
	}
	if c == 0 {
		return 1
	}
	return c
}

func npathMaxInt() int { return int(^uint(0) >> 1) }

func npathAdd(a, b int) int {
	if a < 0 || b < 0 {
		return npathMaxInt()
	}
	max := npathMaxInt()
	if a > max-b {
		return max
	}
	return a + b
}

func npathMul(a, b int) int {
	if a < 0 || b < 0 {
		return npathMaxInt()
	}
	if a == 0 || b == 0 {
		return 0
	}
	max := npathMaxInt()
	if a > max/b {
		return max
	}
	return a * b
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
		npath := npathAdd(expressionComplexity(n.X), 1)
		return npathAdd(npath, npathStmts(n.Body.List))
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
	npath := npathAdd(elsePart, body)
	return npathAdd(npath, expr)
}

// npathFor follows pdepend visitForStatement: 1 + Σ E(loop expressions) +
// NP(body). Init/Cond/Post each contribute their boolean-op complexity.
func npathFor(n *ast.ForStmt) int {
	npath := 1
	npath = npathAdd(npath, expressionComplexity(n.Cond))
	if a, ok := n.Init.(*ast.AssignStmt); ok {
		for _, e := range a.Rhs {
			npath = npathAdd(npath, expressionComplexity(e))
		}
	}
	if a, ok := n.Post.(*ast.AssignStmt); ok {
		for _, e := range a.Rhs {
			npath = npathAdd(npath, expressionComplexity(e))
		}
	}
	return npathAdd(npath, npathStmts(n.Body.List))
}

// npathSwitch follows pdepend visitSwitchStatement: E(tag) plus the sum over
// each case/default label of the NPath of that label's body. There is no
// special handling for a missing default — a default is just another label.
func npathSwitch(body *ast.BlockStmt, tag ast.Expr) int {
	npath := expressionComplexity(tag)
	for _, c := range body.List {
		if cc, ok := c.(*ast.CaseClause); ok {
			npath = npathAdd(npath, npathStmts(cc.Body))
		}
	}
	if npath == 0 {
		return 1
	}
	return npath
}

// npathSelect treats each comm clause like a switch label.
func npathSelect(body *ast.BlockStmt) int {
	npath := 0
	for _, c := range body.List {
		if cc, ok := c.(*ast.CommClause); ok {
			npath = npathAdd(npath, npathStmts(cc.Body))
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
	return NewEffectiveLOCIndex(src).LinesOfCode(fset, start, end)
}

// EffectiveLOCIndex answers effective-lines-of-code queries after one source
// scan. Each prefix entry is the number of code-bearing lines through that
// physical source line.
type EffectiveLOCIndex struct {
	prefix []int
}

// NewEffectiveLOCIndex scans src once and builds an effective-LOC prefix index.
func NewEffectiveLOCIndex(src []byte) *EffectiveLOCIndex {
	lines := splitLines(src)
	index := &EffectiveLOCIndex{prefix: make([]int, len(lines)+1)}
	inBlockComment := false
	for line, raw := range lines {
		hasCode, blockAfter := lineHasCode(raw, inBlockComment)
		inBlockComment = blockAfter
		index.prefix[line+1] = index.prefix[line]
		if hasCode {
			index.prefix[line+1]++
		}
	}
	return index
}

// LinesOfCode returns the effective code-line count in the inclusive source
// span. Positions use physical lines so //line directives do not alter spans.
func (index *EffectiveLOCIndex) LinesOfCode(fset *token.FileSet, start, end token.Pos) int {
	if index == nil {
		return 0
	}
	first := max(fset.PositionFor(start, false).Line, 1)
	lineCount := len(index.prefix) - 1
	last := min(fset.PositionFor(end, false).Line, lineCount)
	if last < first {
		return 0
	}
	return index.prefix[last] - index.prefix[first-1]
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

type commentState = bool

// lineHasCode reports whether a line contains any code outside comments, given
// whether it begins inside a block comment, and returns the block-comment state
// at the line's end.
func lineHasCode(line []byte, inBlock commentState) (hasCode, blockAfter commentState) {
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
