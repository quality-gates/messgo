package model

import (
	"go/ast"
	"go/token"
)

// LocalVariable describes a variable declared inside a function body.
type LocalVariable struct {
	Name   string
	Line   int
	IsLoop bool // declared as a for-init or range loop variable

	ident *ast.Ident
}

// Locals returns the variables declared within this function body: short
// variable declarations (:=), `var` declarations, for-loop counters and range
// variables. The blank identifier, redeclared names, and function-literal
// bodies are skipped. This is the Go analog of pdepend's VariableDeclarator
// nodes.
func Locals(f *Function) []LocalVariable {
	if f == nil || f.Body == nil {
		return nil
	}
	lc := &localCollector{
		fset: f.File.Fset,
		loop: collectLoopIdents(f.Body),
		seen: map[*ast.Ident]bool{},
	}
	ast.Inspect(f.Body, lc.collect)
	return lc.out
}

// LocalRead reports whether v's binding is read in f's body. A closure
// variable that shadows v does not count.
func LocalRead(f *Function, v LocalVariable) bool {
	return bindingRead(f, v.ident)
}

// UnreadParameters returns the named, non-blank parameters of f whose binding
// is never read in its body. A closure parameter or local that shadows one
// does not count as a read. A function without a body has none.
func UnreadParameters(f *Function) []*Parameter {
	if f == nil || f.Body == nil {
		return nil
	}
	var out []*Parameter
	for _, p := range f.Params {
		if p.Name == "" || p.Name == "_" {
			continue
		}
		if !bindingRead(f, p.Ident) {
			out = append(out, p)
		}
	}
	return out
}

// bindingRead uses object identity to distinguish a shadowing declaration from
// a captured binding with the same name, falling back to the name when the
// parser left the identifier unresolved.
func bindingRead(f *Function, target *ast.Ident) bool {
	if f.Body == nil || target == nil {
		return false
	}
	facts := functionIdentifierReadFacts(f)
	if target.Obj != nil {
		return facts.objects[target.Obj]
	}
	return facts.unresolved[target.Name]
}

type localCollector struct {
	fset *token.FileSet
	loop map[*ast.Ident]bool
	seen map[*ast.Ident]bool
	out  []LocalVariable
}

func (lc *localCollector) add(id *ast.Ident) {
	if id == nil || id.Name == "_" || lc.seen[id] {
		return
	}
	lc.seen[id] = true
	lc.out = append(lc.out, LocalVariable{
		Name:   id.Name,
		Line:   lc.fset.Position(id.Pos()).Line,
		IsLoop: lc.loop[id],
		ident:  id,
	})
}

func (lc *localCollector) collect(node ast.Node) bool {
	switch s := node.(type) {
	case *ast.FuncLit:
		return false
	case *ast.AssignStmt:
		for _, id := range defineIdents(s) {
			lc.add(id)
		}
	case *ast.DeclStmt:
		for _, id := range varDeclIdents(s) {
			lc.add(id)
		}
	case *ast.RangeStmt:
		if s.Tok == token.DEFINE {
			lc.add(identOf(s.Key))
			lc.add(identOf(s.Value))
		}
	}
	return true
}

// collectLoopIdents returns the set of identifiers declared as for-loop
// counters (the := init of a ForStmt) or range variables.
func collectLoopIdents(body *ast.BlockStmt) map[*ast.Ident]bool {
	set := map[*ast.Ident]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch fs := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ForStmt:
			if a, ok := fs.Init.(*ast.AssignStmt); ok {
				for _, id := range defineIdents(a) {
					set[id] = true
				}
			}
		case *ast.RangeStmt:
			if fs.Tok == token.DEFINE {
				addLoopIdent(set, fs.Key)
				addLoopIdent(set, fs.Value)
			}
		}
		return true
	})
	return set
}

func addLoopIdent(set map[*ast.Ident]bool, e ast.Expr) {
	if id := identOf(e); id != nil {
		set[id] = true
	}
}

// defineIdents returns the LHS identifiers newly declared by a `:=` assignment.
func defineIdents(a *ast.AssignStmt) []*ast.Ident {
	if a.Tok != token.DEFINE {
		return nil
	}
	var ids []*ast.Ident
	for _, lhs := range a.Lhs {
		id := identOf(lhs)
		if isDeclaredBy(id, a) {
			ids = append(ids, id)
		}
	}
	return ids
}

// varDeclIdents returns the names declared by a `var` declaration statement.
func varDeclIdents(s *ast.DeclStmt) []*ast.Ident {
	gd, ok := s.Decl.(*ast.GenDecl)
	if !ok || gd.Tok != token.VAR {
		return nil
	}
	var ids []*ast.Ident
	for _, spec := range gd.Specs {
		if vs, ok := spec.(*ast.ValueSpec); ok {
			ids = append(ids, vs.Names...)
		}
	}
	return ids
}

func identOf(e ast.Expr) *ast.Ident {
	id, _ := e.(*ast.Ident)
	return id
}

// isDeclaredBy reports whether id's resolved object is declared by decl.
func isDeclaredBy(id *ast.Ident, decl ast.Node) bool {
	return id != nil && id.Obj != nil && id.Obj.Decl == decl
}

type identifierReadFacts struct {
	objects    map[*ast.Object]bool
	unresolved map[string]bool
}

// scanIdentifierReads records every identifier read in body. Identifiers used
// only as assignment, declaration, or range write targets are excluded.
func scanIdentifierReads(body *ast.BlockStmt) identifierReadFacts {
	facts := identifierReadFacts{
		objects:    map[*ast.Object]bool{},
		unresolved: map[string]bool{},
	}
	writes := map[*ast.Ident]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		collectNodeWrites(n, writes)
		if id, ok := n.(*ast.Ident); ok && id.Name != "_" && !writes[id] {
			facts.recordRead(id)
		}
		return true
	})
	return facts
}

func (f identifierReadFacts) recordRead(id *ast.Ident) {
	if id.Obj == nil {
		f.unresolved[id.Name] = true
		return
	}
	f.objects[id.Obj] = true
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

func collectAssignWrites(s *ast.AssignStmt, writes map[*ast.Ident]bool) {
	if s.Tok != token.ASSIGN && s.Tok != token.DEFINE {
		return
	}
	for _, lhs := range s.Lhs {
		if id := identOf(lhs); id != nil {
			writes[id] = true
		}
	}
}

func collectRangeWrites(s *ast.RangeStmt, writes map[*ast.Ident]bool) {
	if s.Tok != token.ASSIGN && s.Tok != token.DEFINE {
		return
	}
	for _, target := range []ast.Expr{s.Key, s.Value} {
		if id := identOf(target); id != nil {
			writes[id] = true
		}
	}
}
