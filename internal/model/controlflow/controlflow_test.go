package controlflow

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// parseBody parses src as the body of func f and returns it with its FileSet.
func parseBody(t *testing.T, body string) (*ast.BlockStmt, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", "package p\n\nfunc f(x int) {\n"+body+"}\n", 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f.Decls[0].(*ast.FuncDecl).Body, fset
}

func line(fset *token.FileSet, p token.Pos) int { return fset.Position(p).Line }

// clauseLines returns the line of each clause's if keyword.
func clauseLines(fset *token.FileSet, c IfChain) []int {
	var lines []int
	for _, cl := range c.Clauses {
		lines = append(lines, line(fset, cl.Pos))
	}
	return lines
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mustOneChain(t *testing.T, body *ast.BlockStmt) IfChain {
	t.Helper()
	chains := IfChains(body)
	if len(chains) != 1 {
		t.Fatalf("IfChains() returned %d chains, want 1", len(chains))
	}
	return chains[0]
}

func TestPlainIf(t *testing.T) {
	body, fset := parseBody(t, `	if x > 0 {
		x++
	}
`)
	c := mustOneChain(t, body)
	if got := clauseLines(fset, c); !equalInts(got, []int{4}) {
		t.Fatalf("clause lines = %v, want [4]", got)
	}
	cl := c.Clauses[0]
	if cl.Init != nil || cl.Cond == nil || len(cl.Body.List) != 1 {
		t.Fatalf("clause = %+v, want no init, a cond and a one-statement body", cl)
	}
	if c.Else != nil || c.ElsePos.IsValid() {
		t.Fatalf("Else = %v, ElsePos = %v, want none", c.Else, c.ElsePos)
	}
}

func TestIfElse(t *testing.T) {
	body, fset := parseBody(t, `	if x > 0 {
		x++
	} else {
		x--
		x--
	}
`)
	c := mustOneChain(t, body)
	if len(c.Clauses) != 1 {
		t.Fatalf("len(Clauses) = %d, want 1", len(c.Clauses))
	}
	if c.Else == nil || len(c.Else.List) != 2 {
		t.Fatalf("Else = %v, want a two-statement block", c.Else)
	}
	if got := line(fset, c.ElsePos); got != 6 {
		t.Fatalf("ElsePos line = %d, want 6", got)
	}
}

func TestElseIfChainIsOneChain(t *testing.T) {
	body, fset := parseBody(t, `	if x > 0 {
		x++
	} else if x < 0 {
		x--
	} else if x == 0 {
		x = 1
	}
`)
	c := mustOneChain(t, body)
	if got := clauseLines(fset, c); !equalInts(got, []int{4, 6, 8}) {
		t.Fatalf("clause lines = %v, want [4 6 8]", got)
	}
	if c.Else != nil {
		t.Fatalf("Else = %v, want nil", c.Else)
	}
	if got := line(fset, c.Clauses[2].Body.Pos()); got != 8 {
		t.Fatalf("third clause body line = %d, want 8", got)
	}
}

func TestElseIfWithInit(t *testing.T) {
	body, fset := parseBody(t, `	if x > 0 {
		x++
	} else if y := x * 2; y < 0 {
		x = y
	} else {
		x = 0
	}
`)
	c := mustOneChain(t, body)
	second := c.Clauses[1]
	if _, ok := second.Init.(*ast.AssignStmt); !ok {
		t.Fatalf("second clause Init = %T, want *ast.AssignStmt", second.Init)
	}
	if _, ok := second.Cond.(*ast.BinaryExpr); !ok {
		t.Fatalf("second clause Cond = %T, want *ast.BinaryExpr", second.Cond)
	}
	if c.Else == nil || line(fset, c.ElsePos) != 8 {
		t.Fatalf("Else at line %d, want 8", line(fset, c.ElsePos))
	}
}

func TestIfInsideTrailingElseStartsNewChain(t *testing.T) {
	body, fset := parseBody(t, `	if x > 0 {
		x++
	} else if x < 0 {
		x--
	} else {
		if x == 0 {
			x = 1
		} else {
			x = 2
		}
	}
`)
	chains := IfChains(body)
	if len(chains) != 2 {
		t.Fatalf("IfChains() returned %d chains, want 2", len(chains))
	}
	if got := clauseLines(fset, chains[0]); !equalInts(got, []int{4, 6}) {
		t.Fatalf("outer clause lines = %v, want [4 6]", got)
	}
	if got := clauseLines(fset, chains[1]); !equalInts(got, []int{9}) {
		t.Fatalf("nested clause lines = %v, want [9]", got)
	}
	if got := line(fset, chains[1].ElsePos); got != 11 {
		t.Fatalf("nested ElsePos line = %d, want 11", got)
	}
}

func TestIfChainsFindsNestedChainsInClauseBodiesAndClosures(t *testing.T) {
	body, _ := parseBody(t, `	if x > 0 {
		if x > 1 {
		}
	} else if x < 0 {
		f := func() {
			if x < -1 {
			}
		}
		f()
	}
`)
	if got := len(IfChains(body)); got != 3 {
		t.Fatalf("IfChains() returned %d chains, want 3", got)
	}
}

func TestIfChainsNilRoot(t *testing.T) {
	if got := IfChains(nil); got != nil {
		t.Fatalf("IfChains(nil) = %v, want nil", got)
	}
}

func TestNewIfChainFromHead(t *testing.T) {
	body, _ := parseBody(t, `	if x > 0 {
	} else if x < 0 {
	}
`)
	head := body.List[0].(*ast.IfStmt)
	c := NewIfChain(head)
	if len(c.Clauses) != 2 || c.Clauses[0].Pos != head.Pos() {
		t.Fatalf("NewIfChain() = %+v, want two clauses starting at the head", c)
	}
}
