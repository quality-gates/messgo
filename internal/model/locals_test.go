package model

import (
	"go/ast"
	"go/token"
	"slices"
	"testing"
)

func parseLocalsFunction(t *testing.T, src string) *Function {
	t.Helper()
	f, err := ParseSource("locals.go", []byte(src))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	if len(f.Functions) == 0 {
		t.Fatal("fixture has no functions")
	}
	return f.Functions[0]
}

func localNamed(t *testing.T, fn *Function, name string) LocalVariable {
	t.Helper()
	for _, v := range Locals(fn) {
		if v.Name == name {
			return v
		}
	}
	t.Fatalf("Locals() = %+v, want %q", Locals(fn), name)
	return LocalVariable{}
}

func localNames(fn *Function) []string {
	var names []string
	for _, v := range Locals(fn) {
		names = append(names, v.Name)
	}
	return names
}

func unreadParameterNames(fn *Function) []string {
	var names []string
	for _, p := range UnreadParameters(fn) {
		names = append(names, p.Name)
	}
	return names
}

func TestLocalsReportsDeclarationsAndReads(t *testing.T) {
	fn := parseLocalsFunction(t, `package sample

var global int

func use(x int) {
	var local int
	local = x
	global = local
	unread := 1
	_ = global
}
`)
	locals := Locals(fn)
	if len(locals) != 2 || locals[0].Name != "local" || locals[0].Line != 6 || locals[1].Name != "unread" || locals[1].Line != 9 {
		t.Fatalf("Locals() = %+v, want local line 6 and unread line 9", locals)
	}
	if got := unreadParameterNames(fn); len(got) != 0 {
		t.Fatalf("UnreadParameters() = %v, want x read", got)
	}
	if !LocalRead(fn, locals[0]) {
		t.Fatal("LocalRead(local) = false, want true")
	}
	if LocalRead(fn, locals[1]) {
		t.Fatal("LocalRead(unread) = true, want false")
	}
}

func TestLocalsMarksLoopVariables(t *testing.T) {
	fn := parseLocalsFunction(t, `package sample

func f(items []int) {
	for i := 0; i < 1; i++ {
	}
	for k, v := range items {
		_, _ = k, v
	}
	n := 0
	_ = n
}
`)
	for _, name := range []string{"i", "k", "v"} {
		if !localNamed(t, fn, name).IsLoop {
			t.Errorf("%s.IsLoop = false, want true", name)
		}
	}
	if localNamed(t, fn, "n").IsLoop {
		t.Error("n.IsLoop = true, want false")
	}
}

func TestLocalsSkipsBlankAndRedeclaredIdentifiers(t *testing.T) {
	fn := parseLocalsFunction(t, `package sample

func pair() (int, error) { return 1, nil }

func f() (n int) {
	n, err := pair()
	_, unused := 1, 2
	for _, item := range []int{} {
		_ = item
	}
	for n = range []int{} {
	}
	_ = err
	return
}
`)
	fn = fn.File.Functions[1]
	if got, want := localNames(fn), []string{"err", "unused", "item"}; !slices.Equal(got, want) {
		t.Fatalf("Locals() names = %v, want %v", got, want)
	}
}

func TestLocalsWithoutBodyOrUnresolvedDeclarations(t *testing.T) {
	if got := Locals(&Function{}); got != nil {
		t.Fatalf("Locals() without body = %+v, want nil", got)
	}
	unresolved := &Function{File: &File{Fset: token.NewFileSet()}, Body: &ast.BlockStmt{List: []ast.Stmt{
		&ast.AssignStmt{Tok: token.DEFINE, Lhs: []ast.Expr{&ast.Ident{Name: "x"}}, Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}}},
	}}}
	if got := Locals(unresolved); len(got) != 0 {
		t.Fatalf("Locals() = %+v, want none when the declaration is unresolved", got)
	}
}

func TestFunctionLiteralHasItsOwnLocalScope(t *testing.T) {
	fn := parseLocalsFunction(t, `package sample

func outer(x int) {
	closure := func() {
		x := 1
		_ = x
	}
	_ = closure
}
`)
	if got, want := localNames(fn), []string{"closure"}; !slices.Equal(got, want) {
		t.Fatalf("Locals() names = %v, want %v", got, want)
	}
	if got, want := unreadParameterNames(fn), []string{"x"}; !slices.Equal(got, want) {
		t.Fatalf("UnreadParameters() = %v, want %v: the closure's shadowed x is not a read of the outer parameter", got, want)
	}
}

func TestUnreadParametersCountsCapturedOuterVariable(t *testing.T) {
	fn := parseLocalsFunction(t, `package sample

func outer(x int) {
	closure := func() int {
		return x
	}
	_ = closure
}
`)
	if got := unreadParameterNames(fn); len(got) != 0 {
		t.Fatalf("UnreadParameters() = %v, want the captured outer parameter read", got)
	}
}

func TestReadQueriesFallBackToNamesForUnresolvedIdentifiers(t *testing.T) {
	fn := &Function{
		Params: []*Parameter{
			{Name: "external", Ident: &ast.Ident{Name: "external"}},
			{Name: "writeOnly", Ident: &ast.Ident{Name: "writeOnly"}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ExprStmt{X: &ast.Ident{Name: "external"}},
			&ast.AssignStmt{Lhs: []ast.Expr{&ast.Ident{Name: "writeOnly"}}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}}},
		}},
	}
	if got, want := unreadParameterNames(fn), []string{"writeOnly"}; !slices.Equal(got, want) {
		t.Fatalf("UnreadParameters() = %v, want %v", got, want)
	}
	if !LocalRead(fn, LocalVariable{Name: "external", ident: &ast.Ident{Name: "external"}}) {
		t.Fatal("LocalRead() missed an unresolved read with the same name")
	}
}

func TestReadQueriesRejectMissingBindings(t *testing.T) {
	fn := parseLocalsFunction(t, `package sample

func f(x int) { _ = x }
`)
	if LocalRead(fn, LocalVariable{Name: "x"}) {
		t.Fatal("LocalRead() without a binding = true, want false")
	}
	if LocalRead(&Function{}, LocalVariable{Name: "x", ident: &ast.Ident{Name: "x"}}) {
		t.Fatal("LocalRead() on a function without a body = true, want false")
	}
}

func TestUnreadParametersSkipsBlankUnnamedAndBodilessFunctions(t *testing.T) {
	f, err := ParseSource("locals.go", []byte(`package sample

func blank(_ int) {}

func unnamed(int) {}

func unread(a, b int) { _ = b }

func external(x int)
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	for _, fn := range f.Functions[:2] {
		if got := unreadParameterNames(fn); len(got) != 0 {
			t.Errorf("UnreadParameters(%s) = %v, want none", fn.Name, got)
		}
	}
	if got, want := unreadParameterNames(f.Functions[2]), []string{"a"}; !slices.Equal(got, want) {
		t.Errorf("UnreadParameters(unread) = %v, want %v", got, want)
	}
	if got := UnreadParameters(f.Functions[3]); got != nil {
		t.Errorf("UnreadParameters(external) = %v, want nil for a function without a body", got)
	}
	if got := UnreadParameters(nil); got != nil {
		t.Errorf("UnreadParameters(nil) = %v, want nil", got)
	}
}

func TestReadQueriesTrackEveryClosureDeclarationKind(t *testing.T) {
	fn := parseLocalsFunction(t, `package sample

func outer(captured, ignored int) (named int) {
	outerLocal := 0
	index := 0
	_ = outerLocal
	closure := func(arg int) (result int) {
		var declared int
		const constant = 1
		type local struct{}
		short := 2
		declared = 3
		ignored, reused := captured, 4
		existing := 0
		existing, reusedExisting := existing, 5
		for key, value := range []int{1} {
			_ = key
			_ = value
		}
		for single := range []int{1} {
			_ = single
		}
		for index = range []int{1} {
		}
		nested := func(inner int) int {
			return inner
		}
		_ = arg
		_ = declared
		_ = constant
		_ = local{}
		_ = short
		_ = reused
		_ = ignored
		_ = nested
		_ = result
		return result
	}
	_ = closure
	return named
}
`)
	if got, want := localNames(fn), []string{"outerLocal", "index", "closure"}; !slices.Equal(got, want) {
		t.Fatalf("Locals() names = %v, want %v", got, want)
	}
	if got, want := unreadParameterNames(fn), []string{"ignored"}; !slices.Equal(got, want) {
		t.Errorf("UnreadParameters() = %v, want %v: captured is read, ignored is only shadowed", got, want)
	}
	if !LocalRead(fn, localNamed(t, fn, "outerLocal")) || !LocalRead(fn, localNamed(t, fn, "closure")) {
		t.Error("LocalRead() missed an outer local read")
	}
	if LocalRead(fn, localNamed(t, fn, "index")) {
		t.Error("LocalRead(index) = true, a range assignment is a write, not a read")
	}
}
