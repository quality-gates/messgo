package util

import (
	"go/ast"
	"go/token"
)

// LocalVar is a variable declared inside a function body.
type LocalVar struct {
	Name   string
	Ident  *ast.Ident
	Line   int
	IsLoop bool // declared as a for-init or range loop variable
}

// LocalVariables collects the variables declared within a function body:
// short variable declarations (:=), `var` declarations, for-loop counters and
// range variables. The blank identifier is skipped. This is the Go analog of
// pdepend's VariableDeclarator nodes.
func LocalVariables(body *ast.BlockStmt, fset *token.FileSet) []LocalVar {
	lc := &localCollector{
		fset: fset,
		loop: collectLoopIdents(body),
		seen: map[*ast.Ident]bool{},
	}
	ast.Inspect(body, lc.collect)
	return lc.out
}

type localCollector struct {
	fset *token.FileSet
	loop map[*ast.Ident]bool
	seen map[*ast.Ident]bool
	out  []LocalVar
}

func (lc *localCollector) add(id *ast.Ident) {
	if id == nil || id.Name == "_" || lc.seen[id] {
		return
	}
	lc.seen[id] = true
	lc.out = append(lc.out, LocalVar{
		Name:   id.Name,
		Ident:  id,
		Line:   lc.fset.Position(id.Pos()).Line,
		IsLoop: lc.loop[id],
	})
}

func (lc *localCollector) collect(node ast.Node) bool {
	switch s := node.(type) {
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
// counters (the := init of a ForStmt), which callers may treat specially.
func collectLoopIdents(body *ast.BlockStmt) map[*ast.Ident]bool {
	set := map[*ast.Ident]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if fs, ok := n.(*ast.ForStmt); ok {
			if a, ok := fs.Init.(*ast.AssignStmt); ok {
				for _, id := range defineIdents(a) {
					set[id] = true
				}
			}
		}
		return true
	})
	return set
}

// defineIdents returns the LHS identifiers of a `:=` assignment.
func defineIdents(a *ast.AssignStmt) []*ast.Ident {
	if a.Tok != token.DEFINE {
		return nil
	}
	var ids []*ast.Ident
	for _, lhs := range a.Lhs {
		if id := identOf(lhs); id != nil {
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

// FindCalls returns all call expressions in a node whose callee renders to one
// of the given function names (matched against the textual call expression,
// e.g. "fmt.Println" or "panic").
type Call struct {
	Expr *ast.CallExpr
	Name string // dotted name of the callee
	Line int
}

// Calls returns every call expression within n along with its dotted callee
// name and line.
func Calls(n ast.Node, fset *token.FileSet) []Call {
	var out []Call
	ast.Inspect(n, func(node ast.Node) bool {
		if ce, ok := node.(*ast.CallExpr); ok {
			out = append(out, Call{Expr: ce, Name: CalleeName(ce.Fun), Line: fset.Position(ce.Pos()).Line})
		}
		return true
	})
	return out
}

// CalleeName renders a call's function expression to a dotted name, e.g.
// "fmt.Println", "os.Exit", "panic". Returns "" if it can't be expressed.
func CalleeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if x := CalleeName(t.X); x != "" {
			return x + "." + t.Sel.Name
		}
		return t.Sel.Name
	case *ast.ParenExpr:
		return CalleeName(t.X)
	}
	return ""
}
