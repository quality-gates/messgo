package model

import (
	"go/ast"
	"go/constant"
	"go/token"
	"maps"

	"github.com/quality-gates/messgo/internal/util"
)

// Call describes a function call found inside an artifact.
type Call struct {
	Name string
	Line int
}

// SourcePosition identifies a position in the original source file.
type SourcePosition struct {
	Line   int
	Column int
}

// DuplicateLiteralKey describes a repeated constant key in a composite literal.
type DuplicateLiteralKey struct {
	Display   string
	FirstLine int
	Line      int
}

// PackageVar describes a package-level variable declaration.
type PackageVar struct {
	Name string
	Line int
}

// LocalVariable describes a local variable declaration.
type LocalVariable struct {
	Name   string
	Line   int
	IsLoop bool
	Ident  *ast.Ident
}

// HasGoto reports whether the function body contains a goto statement.
func (f *Function) HasGoto() bool {
	if f.Body == nil {
		return false
	}
	found := false
	ast.Inspect(f.Body, func(n ast.Node) bool {
		if b, ok := n.(*ast.BranchStmt); ok && b.Tok == token.GOTO {
			found = true
			return false
		}
		return true
	})
	return found
}

// Calls returns every call expression in this function body.
func (f *Function) Calls() []Call {
	if f.Body == nil {
		return nil
	}
	var out []Call
	ast.Inspect(f.Body, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			out = append(out, Call{Name: calleeName(ce.Fun), Line: f.File.Fset.Position(ce.Pos()).Line})
		}
		return true
	})
	return out
}

// LoopConditionCalls returns calls to selected names found in for-loop
// conditions, reported at the loop's line.
func (f *Function) LoopConditionCalls(names map[string]bool) []Call {
	if f.Body == nil {
		return nil
	}
	var out []Call
	ast.Inspect(f.Body, func(n ast.Node) bool {
		fs, ok := n.(*ast.ForStmt)
		if !ok || fs.Cond == nil {
			return true
		}
		ast.Inspect(fs.Cond, func(cn ast.Node) bool {
			ce, ok := cn.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := calleeName(ce.Fun)
			if names[name] {
				out = append(out, Call{Name: name, Line: f.File.Fset.Position(fs.Pos()).Line})
			}
			return true
		})
		return true
	})
	return out
}

// EmptyNilCheckBlockLines returns lines for empty if-blocks whose condition
// compares any operand with nil using !=.
func (f *Function) EmptyNilCheckBlockLines() []int {
	if f.Body == nil {
		return nil
	}
	var lines []int
	ast.Inspect(f.Body, func(n ast.Node) bool {
		ifs, ok := n.(*ast.IfStmt)
		if !ok || ifs.Body == nil || len(ifs.Body.List) != 0 {
			return true
		}
		if conditionChecksNil(ifs.Cond) {
			lines = append(lines, f.File.Fset.Position(ifs.Pos()).Line)
		}
		return true
	})
	return lines
}

// ElseBlockLines returns the source lines of else blocks, excluding else-if
// chains.
func (f *Function) ElseBlockLines() []int {
	if f.Body == nil {
		return nil
	}
	var lines []int
	ast.Inspect(f.Body, func(n ast.Node) bool {
		ifs, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if _, isBlock := ifs.Else.(*ast.BlockStmt); isBlock {
			lines = append(lines, f.File.Fset.Position(ifs.Else.Pos()).Line)
		}
		return true
	})
	return lines
}

// IfAssignmentInitPositions returns positions for plain assignment initializers
// in if statements. Short declarations are intentionally excluded.
func (f *Function) IfAssignmentInitPositions() []SourcePosition {
	if f.Body == nil {
		return nil
	}
	var positions []SourcePosition
	ast.Inspect(f.Body, func(n ast.Node) bool {
		ifs, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		assign, ok := ifs.Init.(*ast.AssignStmt)
		if ok && assign.Tok == token.ASSIGN {
			pos := f.File.Fset.Position(assign.Pos())
			positions = append(positions, SourcePosition{Line: pos.Line, Column: pos.Column})
		}
		return true
	})
	return positions
}

// DuplicateLiteralKeys returns duplicate constant keys in composite literals.
func (f *Function) DuplicateLiteralKeys() []DuplicateLiteralKey {
	if f.Body == nil {
		return nil
	}
	return duplicateLiteralKeys(f.Body, f.File.Fset)
}

// PackageDuplicateLiteralKeys returns duplicate constant keys in package-level
// composite literals.
func (f *File) PackageDuplicateLiteralKeys() []DuplicateLiteralKey {
	var out []DuplicateLiteralKey
	for _, decl := range f.Syntax.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		out = append(out, duplicateLiteralKeys(gd, f.Fset)...)
	}
	return out
}

func duplicateLiteralKeys(root ast.Node, fset *token.FileSet) []DuplicateLiteralKey {
	var out []DuplicateLiteralKey
	ast.Inspect(root, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		out = append(out, duplicateKeysInLiteral(cl, fset)...)
		return true
	})
	return out
}

func duplicateKeysInLiteral(cl *ast.CompositeLit, fset *token.FileSet) []DuplicateLiteralKey {
	var out []DuplicateLiteralKey
	seen := map[string]int{}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := literalKey(kv.Key)
		if !ok {
			continue
		}
		line := fset.Position(kv.Key.Pos()).Line
		if first, dup := seen[key]; dup {
			out = append(out, DuplicateLiteralKey{
				Display:   displayKey(kv.Key),
				FirstLine: first,
				Line:      line,
			})
			continue
		}
		seen[key] = line
	}
	return out
}

// PackageVars returns package-level variables declared in the file, skipping
// the blank identifier.
func (f *File) PackageVars() []PackageVar {
	var out []PackageVar
	for _, decl := range f.Syntax.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if name.Name == "_" {
					continue
				}
				out = append(out, PackageVar{Name: name.Name, Line: f.Fset.Position(name.Pos()).Line})
			}
		}
	}
	return out
}

// MutatedPackageGlobals returns package-level variable names mutated in this
// file unless the runner has already populated cross-file mutation data.
func (f *File) MutatedPackageGlobals() map[string]bool {
	if f.MutatedGlobals != nil {
		return f.MutatedGlobals
	}
	return util.MutatedGlobalNames([]*ast.File{f.Syntax})
}

// LocalVariables returns local variable declarations in this function.
func (f *Function) LocalVariables() []LocalVariable {
	if f.Body == nil {
		return nil
	}
	locals := util.LocalVariables(f.Body, f.File.Fset)
	out := make([]LocalVariable, 0, len(locals))
	for _, v := range locals {
		out = append(out, LocalVariable{Name: v.Name, Line: v.Line, IsLoop: v.IsLoop, Ident: v.Ident})
	}
	return out
}

// IdentifierReads returns names read from this function body. Identifiers used
// only as assignment or declaration write targets are excluded.
func (f *Function) IdentifierReads() map[string]bool {
	return maps.Clone(functionIdentifierReadFacts(f).names)
}

// IdentifierRead reports whether target's binding is read in this function.
// Object identity distinguishes a shadowing local from a captured parameter or
// local with the same name.
func (f *Function) IdentifierRead(target *ast.Ident) bool {
	if f.Body == nil || target == nil {
		return false
	}
	facts := functionIdentifierReadFacts(f)
	if target.Obj != nil {
		return facts.objects[target.Obj]
	}
	return facts.unresolved[target.Name]
}

type identifierReadFacts struct {
	names       map[string]bool
	objects     map[*ast.Object]bool
	unresolved  map[string]bool
	writes      map[*ast.Ident]bool
	objectDepth map[*ast.Object]int
}

type identifierReadVisitor struct {
	facts *identifierReadFacts
	depth int
}

func scanIdentifierReads(body *ast.BlockStmt) identifierReadFacts {
	facts := identifierReadFacts{
		names:       map[string]bool{},
		objects:     map[*ast.Object]bool{},
		unresolved:  map[string]bool{},
		writes:      map[*ast.Ident]bool{},
		objectDepth: map[*ast.Object]int{},
	}
	if body != nil {
		ast.Walk(identifierReadVisitor{facts: &facts}, body)
	}
	return facts
}

func (v identifierReadVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}
	depth := v.depth
	if literal, ok := node.(*ast.FuncLit); ok {
		depth++
		collectFieldObjectDepth(literal.Type.Params, depth, v.facts.objectDepth)
		collectFieldObjectDepth(literal.Type.Results, depth, v.facts.objectDepth)
	}
	collectDeclaredObjectDepth(node, depth, v.facts.objectDepth)
	collectNodeWrites(node, v.facts.writes)
	if id, ok := node.(*ast.Ident); ok {
		v.facts.recordRead(id)
	}
	return identifierReadVisitor{facts: v.facts, depth: depth}
}

func (f *identifierReadFacts) recordRead(id *ast.Ident) {
	if !isReadIdentifier(id, f.writes) {
		return
	}
	if id.Obj == nil {
		f.unresolved[id.Name] = true
		f.names[id.Name] = true
		return
	}
	f.objects[id.Obj] = true
	if f.objectDepth[id.Obj] == 0 {
		f.names[id.Name] = true
	}
}

func collectNodeWrites(node ast.Node, writes map[*ast.Ident]bool) {
	switch declaration := node.(type) {
	case *ast.AssignStmt:
		collectAssignWrites(declaration, writes)
	case *ast.ValueSpec:
		for _, id := range declaration.Names {
			writes[id] = true
		}
	case *ast.RangeStmt:
		collectRangeWrites(declaration, writes)
	}
}

func collectDeclaredObjectDepth(node ast.Node, depth int, objectDepth map[*ast.Object]int) {
	switch declaration := node.(type) {
	case *ast.ValueSpec:
		collectIdentObjectDepth(declaration.Names, depth, objectDepth)
	case *ast.AssignStmt:
		collectShortObjectDepth(declaration, depth, objectDepth)
	case *ast.RangeStmt:
		collectRangeObjectDepth(declaration, depth, objectDepth)
	case *ast.TypeSpec:
		setObjectDepth(declaration.Name, depth, objectDepth)
	}
}

func collectFieldObjectDepth(fields *ast.FieldList, depth int, objectDepth map[*ast.Object]int) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		collectIdentObjectDepth(field.Names, depth, objectDepth)
	}
}

func collectIdentObjectDepth(idents []*ast.Ident, depth int, objectDepth map[*ast.Object]int) {
	for _, id := range idents {
		setObjectDepth(id, depth, objectDepth)
	}
}

func setObjectDepth(id *ast.Ident, depth int, objectDepth map[*ast.Object]int) {
	if id != nil && id.Obj != nil {
		objectDepth[id.Obj] = depth
	}
}

func collectShortObjectDepth(assign *ast.AssignStmt, depth int, objectDepth map[*ast.Object]int) {
	if assign.Tok != token.DEFINE {
		return
	}
	for _, lhs := range assign.Lhs {
		id, ok := lhs.(*ast.Ident)
		if ok && isDeclaredBy(id, assign) {
			setObjectDepth(id, depth, objectDepth)
		}
	}
}

func collectRangeObjectDepth(rng *ast.RangeStmt, depth int, objectDepth map[*ast.Object]int) {
	if rng.Tok != token.DEFINE {
		return
	}
	for _, expr := range []ast.Expr{rng.Key, rng.Value} {
		id, ok := expr.(*ast.Ident)
		if ok && isRangeDeclaredBy(id, rng) {
			setObjectDepth(id, depth, objectDepth)
		}
	}
}

func isReadIdentifier(id *ast.Ident, writeIdents map[*ast.Ident]bool) bool {
	return id.Name != "_" && !writeIdents[id]
}

func isDeclaredBy(id *ast.Ident, decl ast.Node) bool {
	return id != nil && id.Obj != nil && id.Obj.Decl == decl
}

func isRangeDeclaredBy(id *ast.Ident, decl *ast.RangeStmt) bool {
	if !rangeDeclHasIdentifier(id, decl) {
		return false
	}
	assign, ok := id.Obj.Decl.(*ast.AssignStmt)
	if !ok || assign.Tok != token.DEFINE {
		return false
	}
	return assignmentContainsIdent(assign, id)
}

func rangeDeclHasIdentifier(id *ast.Ident, decl *ast.RangeStmt) bool {
	if id == nil || id.Obj == nil {
		return false
	}
	if decl == nil || decl.Tok != token.DEFINE {
		return false
	}
	return id == decl.Key || id == decl.Value
}

func assignmentContainsIdent(assign *ast.AssignStmt, id *ast.Ident) bool {
	for _, lhs := range assign.Lhs {
		if lhs == id {
			return true
		}
	}
	return false
}

func collectAssignWrites(s *ast.AssignStmt, writeIdents map[*ast.Ident]bool) {
	if s.Tok == token.ASSIGN || s.Tok == token.DEFINE {
		for _, lhs := range s.Lhs {
			if id, ok := lhs.(*ast.Ident); ok {
				writeIdents[id] = true
			}
		}
	}
}

func collectRangeWrites(s *ast.RangeStmt, writeIdents map[*ast.Ident]bool) {
	if s.Tok == token.ASSIGN || s.Tok == token.DEFINE {
		if id, ok := s.Key.(*ast.Ident); ok {
			writeIdents[id] = true
		}
		if id, ok := s.Value.(*ast.Ident); ok {
			writeIdents[id] = true
		}
	}
}

// AccessorField returns the field wrapped by a trivial getter or setter.
func (f *Function) AccessorField(fields map[string]bool) string {
	if f.Body == nil || len(f.Body.List) != 1 {
		return ""
	}
	switch stmt := f.Body.List[0].(type) {
	case *ast.ReturnStmt:
		return returnAccessorField(stmt, f.RecvName, fields)
	case *ast.AssignStmt:
		return assignAccessorField(stmt, f.RecvName, fields)
	}
	return ""
}

func returnAccessorField(stmt *ast.ReturnStmt, recvName string, fields map[string]bool) string {
	if len(stmt.Results) != 1 {
		return ""
	}
	return receiverFieldSelector(stmt.Results[0], recvName, fields)
}

func assignAccessorField(stmt *ast.AssignStmt, recvName string, fields map[string]bool) string {
	if stmt.Tok != token.ASSIGN || len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
		return ""
	}
	if !isPlainValue(stmt.Rhs[0]) {
		return ""
	}
	return receiverFieldSelector(stmt.Lhs[0], recvName, fields)
}

// ReceiverUses returns fields and sibling methods selected through the
// receiver variable in this function body.
func (f *Function) ReceiverUses(fields map[string]bool, methods map[string]int) (usedFields, calledMethods []string) {
	if f.Body == nil || f.RecvName == "" || f.RecvName == "_" {
		return nil, nil
	}
	seen := map[string]bool{}
	ast.Inspect(f.Body, func(n ast.Node) bool {
		name := receiverSelector(n, f.RecvName)
		if name == "" || seen[name] {
			return true
		}
		_, isMethod := methods[name]
		switch {
		case fields[name]:
			seen[name] = true
			usedFields = append(usedFields, name)
		case isMethod:
			seen[name] = true
			calledMethods = append(calledMethods, name)
		}
		return true
	})
	return usedFields, calledMethods
}

func isPlainValue(e ast.Expr) bool {
	switch e.(type) {
	case *ast.Ident, *ast.BasicLit:
		return true
	}
	return false
}

func receiverFieldSelector(e ast.Expr, recvName string, fields map[string]bool) string {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	id, ok := sel.X.(*ast.Ident)
	if ok && id.Name == recvName && fields[sel.Sel.Name] {
		return sel.Sel.Name
	}
	return ""
}

func receiverSelector(n ast.Node, recvName string) string {
	sel, ok := n.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != recvName {
		return ""
	}
	return sel.Sel.Name
}

func conditionChecksNil(cond ast.Expr) bool {
	found := false
	ast.Inspect(cond, func(n ast.Node) bool {
		be, ok := n.(*ast.BinaryExpr)
		if !ok || be.Op != token.NEQ {
			return true
		}
		if isNilIdent(be.X) || isNilIdent(be.Y) {
			found = true
		}
		return true
	})
	return found
}

func isNilIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

func literalKey(e ast.Expr) (string, bool) {
	switch k := e.(type) {
	case *ast.ParenExpr:
		return literalKey(k.X)
	case *ast.UnaryExpr:
		return unaryLitKey(k)
	case *ast.BasicLit:
		return basicLitKey(k.Value, k.Kind)
	case *ast.Ident:
		return "ident:" + k.Name, true
	}
	return "", false
}

func basicLitKey(val string, kind token.Token) (string, bool) {
	v := constant.MakeFromLiteral(val, kind, 0)
	if v.Kind() == constant.Unknown {
		return kind.String() + ":" + val, true
	}
	return v.Kind().String() + ":" + v.ExactString(), true
}

func unaryLitKey(u *ast.UnaryExpr) (string, bool) {
	if u.Op != token.SUB && u.Op != token.ADD {
		return "", false
	}
	lit, ok := u.X.(*ast.BasicLit)
	if !ok {
		return "", false
	}
	return basicLitKey(u.Op.String()+lit.Value, lit.Kind)
}

func displayKey(e ast.Expr) string {
	switch k := e.(type) {
	case *ast.ParenExpr:
		return displayKey(k.X)
	case *ast.UnaryExpr:
		if k.Op == token.SUB || k.Op == token.ADD {
			return k.Op.String() + displayKey(k.X)
		}
	case *ast.BasicLit:
		return k.Value
	case *ast.Ident:
		return k.Name
	}
	return ""
}

func calleeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		x := calleeName(t.X)
		if x == "" {
			return t.Sel.Name
		}
		return x + "." + t.Sel.Name
	case *ast.ParenExpr:
		return calleeName(t.X)
	default:
		return ""
	}
}
