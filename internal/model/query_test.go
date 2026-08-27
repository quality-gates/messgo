package model

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestFunctionHasGoto(t *testing.T) {
	src := []byte(`package sample

func jump(flag bool) {
	if flag {
		goto done
	}
done:
	return
}

func stay() {
	return
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	if !f.Functions[0].HasGoto() {
		t.Fatalf("%s HasGoto() = false, want true", f.Functions[0].Name)
	}
	if f.Functions[1].HasGoto() {
		t.Fatalf("%s HasGoto() = true, want false", f.Functions[1].Name)
	}
}

func TestFunctionLoopConditionCalls(t *testing.T) {
	src := []byte(`package sample

func scan(items []int) {
	for i := 0; i < len(items); i++ {
		_ = i
	}
	for cap(items) > 0 {
		break
	}
	for _, item := range items {
		_ = item
	}
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	calls := f.Functions[0].LoopConditionCalls(map[string]bool{"len": true, "cap": true})
	if len(calls) != 2 {
		t.Fatalf("LoopConditionCalls count = %d, want 2", len(calls))
	}
	if calls[0].Name != "len" || calls[0].Line != 4 {
		t.Fatalf("first call = %+v, want len at line 4", calls[0])
	}
	if calls[1].Name != "cap" || calls[1].Line != 7 {
		t.Fatalf("second call = %+v, want cap at line 7", calls[1])
	}
}

func TestFunctionCalls(t *testing.T) {
	src := []byte(`package sample

func exit() {
	os.Exit(1)
	syscall.Exit(1)
	println("debug")
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	calls := f.Functions[0].Calls()
	want := []Call{
		{Name: "os.Exit", Line: 4},
		{Name: "syscall.Exit", Line: 5},
		{Name: "println", Line: 6},
	}
	if len(calls) != len(want) {
		t.Fatalf("Calls() = %+v, want %+v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("Calls()[%d] = %+v, want %+v", i, calls[i], want[i])
		}
	}
}

func TestFunctionStatementQueriesReportSourceLines(t *testing.T) {
	src := []byte(`package sample

func inspect(err error, xs []int) {
	if err != nil {
	} else {
		_ = err
	}
	if value = len(xs); value > 0 {
		_ = value
	}
	_ = map[string]int{"a": 1, "a": 2}
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	fn := f.Functions[0]

	if got := fn.EmptyNilCheckBlockLines(); len(got) != 1 || got[0] != 4 {
		t.Fatalf("EmptyNilCheckBlockLines() = %v, want [4]", got)
	}
	if got := fn.ElseBlockLines(); len(got) != 1 || got[0] != 5 {
		t.Fatalf("ElseBlockLines() = %v, want [5]", got)
	}
	assigns := fn.IfAssignmentInitPositions()
	if len(assigns) != 1 || assigns[0].Line != 8 || assigns[0].Column != 5 {
		t.Fatalf("IfAssignmentInitPositions() = %+v, want line 8 column 5", assigns)
	}
	dups := fn.DuplicateLiteralKeys()
	if len(dups) != 1 || dups[0].Display != `"a"` || dups[0].FirstLine != 11 || dups[0].Line != 11 {
		t.Fatalf("DuplicateLiteralKeys() = %+v, want duplicate string key on line 11", dups)
	}
}

func TestFileAndIdentifierQueries(t *testing.T) {
	src := []byte(`package sample

var global int

type item struct {
	used int
}

func use(x int) {
	var local int
	_ = item{used: x}
	local = x
	global = local
	unread := 1
	_ = global
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	vars := f.PackageVars()
	if len(vars) != 1 || vars[0].Name != "global" || vars[0].Line != 3 {
		t.Fatalf("PackageVars() = %+v, want global at line 3", vars)
	}
	mutated := f.MutatedPackageGlobals()
	if !mutated["global"] {
		t.Fatalf("MutatedPackageGlobals() = %v, want global", mutated)
	}
	selected := f.SelectedMemberNames()
	if !selected["used"] {
		t.Fatalf("SelectedMemberNames() = %v, want used", selected)
	}
	fn := f.Functions[0]
	locals := fn.LocalVariables()
	if len(locals) != 2 || locals[0].Name != "local" || locals[0].Line != 10 || locals[1].Name != "unread" || locals[1].Line != 14 {
		t.Fatalf("LocalVariables() = %+v, want local line 10 and unread line 14", locals)
	}
	reads := fn.IdentifierReads()
	if !reads["x"] || !reads["local"] || reads["unread"] {
		t.Fatalf("IdentifierReads() = %v, want x/local read and unread not read", reads)
	}
}

func TestSelectedMemberNamesIgnoresMapKeys(t *testing.T) {
	f, err := ParseSource("query.go", []byte(`package sample
const key = 1
type T struct { key int }
func f() { _ = map[int]int{key: 2} }
`))
	if err != nil {
		t.Fatal(err)
	}
	if f.SelectedMemberNames()["key"] {
		t.Fatal("map key counted as a struct member use")
	}
}

func TestDuplicateLiteralKeysUseConstantValue(t *testing.T) {
	f, err := ParseSource("query.go", []byte("package sample\nfunc f() {\n\t_ = map[int]int{1: 0, 01: 0}\n\t_ = map[string]int{\"a\": 0, `a`: 0}\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Functions[0].DuplicateLiteralKeys(); len(got) < 2 {
		t.Fatalf("DuplicateLiteralKeys() = %#v, want 1/01 and a/`a`", got)
	}
}

func TestEmptyNilCheckRequiresInequality(t *testing.T) {
	f, err := ParseSource("query.go", []byte(`package sample
func f(pointer *int) {
	if pointer == nil {}
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Functions[0].EmptyNilCheckBlockLines(); len(got) != 0 {
		t.Fatalf("EmptyNilCheckBlockLines() = %v, want none for == nil", got)
	}
}

func TestFunctionLiteralHasItsOwnLocalScope(t *testing.T) {
	src := []byte(`package sample

func outer(x int) {
	closure := func() {
		x := 1
		_ = x
	}
	_ = closure
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	fn := f.Functions[0]
	locals := fn.LocalVariables()
	if len(locals) != 1 || locals[0].Name != "closure" {
		t.Fatalf("LocalVariables() = %+v, want only closure from the outer function", locals)
	}
	if reads := fn.IdentifierReads(); reads["x"] {
		t.Fatalf("IdentifierReads() = %v, inner shadow should not count as an outer read", reads)
	}
	if fn.IdentifierRead(fn.Params[0].Ident) {
		t.Fatal("IdentifierRead() counted the closure's shadowed x as a read of the outer parameter")
	}
}

func TestIdentifierReadsIncludesCapturedOuterVariable(t *testing.T) {
	src := []byte(`package sample

func outer(x int) {
	closure := func() int {
		return x
	}
	_ = closure
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	if reads := f.Functions[0].IdentifierReads(); !reads["x"] {
		t.Fatalf("IdentifierReads() = %v, captured outer variable x should count as read", reads)
	}
	if !f.Functions[0].IdentifierRead(f.Functions[0].Params[0].Ident) {
		t.Fatal("IdentifierRead() missed the captured outer parameter")
	}
}

func TestIdentifierReadFallsBackForUnresolvedIdentifiers(t *testing.T) {
	read := &ast.Ident{Name: "external"}
	write := &ast.Ident{Name: "writeOnly"}
	fn := &Function{Body: &ast.BlockStmt{List: []ast.Stmt{
		&ast.ExprStmt{X: read},
		&ast.AssignStmt{Lhs: []ast.Expr{write}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}}},
	}}}

	if !fn.IdentifierRead(&ast.Ident{Name: "external"}) {
		t.Fatal("IdentifierRead() missed an unresolved read with the same name")
	}
	if fn.IdentifierRead(&ast.Ident{Name: "writeOnly"}) {
		t.Fatal("IdentifierRead() counted an unresolved write target as a read")
	}
}

func TestIdentifierQueriesTrackEveryClosureDeclarationKind(t *testing.T) {
	src := []byte(`package sample

func outer(captured int) (named int) {
	outerLocal := 0
	index := 0
	_ = outerLocal
	closure := func(arg int) (result int) {
		var declared int
		const constant = 1
		type local struct{}
		short := 2
		declared = 3
		captured, reused := captured, 4
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
		_ = nested
		_ = result
		return result
	}
	_ = closure
	return named
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	fn := f.Functions[0]
	locals := fn.LocalVariables()
	if len(locals) != 3 || locals[0].Name != "outerLocal" || locals[1].Name != "index" || locals[2].Name != "closure" {
		t.Fatalf("LocalVariables() = %+v, want only outerLocal, index, and closure", locals)
	}

	reads := fn.IdentifierReads()
	for _, name := range []string{"captured", "outerLocal", "closure", "named"} {
		if !reads[name] {
			t.Errorf("IdentifierReads() = %v, want outer %q to be read", reads, name)
		}
	}
	for _, name := range []string{"arg", "result", "declared", "constant", "local", "short", "reused", "existing", "reusedExisting", "key", "value", "single", "nested", "inner", "index"} {
		if reads[name] {
			t.Errorf("IdentifierReads() = %v, inner/write-only %q should not be reported as an outer read", reads, name)
		}
	}
	if !fn.IdentifierRead(fn.Params[0].Ident) {
		t.Fatal("IdentifierRead() missed the captured outer parameter")
	}
	if !fn.IdentifierRead(locals[0].Ident) {
		t.Fatal("IdentifierRead() missed the outer local read")
	}
	if fn.IdentifierRead(nil) {
		t.Fatal("IdentifierRead(nil) = true, want false")
	}
}

func TestFunctionReceiverQueries(t *testing.T) {
	src := []byte(`package sample

type counter struct {
	value int
	other int
}

func (c *counter) Value() int {
	return c.value
}

func (c *counter) SetValue(v int) {
	c.value = v
}

func (c *counter) Touch() {
	c.value++
	c.SetValue(c.other)
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	class := f.Classes[0]
	fields := map[string]bool{"value": true, "other": true}
	methods := map[string]int{}
	for i, method := range class.Methods {
		methods[method.Name] = i
	}

	if got := class.Methods[0].AccessorField(fields); got != "value" {
		t.Fatalf("Value AccessorField() = %q, want value", got)
	}
	if got := class.Methods[1].AccessorField(fields); got != "value" {
		t.Fatalf("SetValue AccessorField() = %q, want value", got)
	}
	usedFields, calledMethods := class.Methods[2].ReceiverUses(fields, methods)
	if len(usedFields) != 2 || usedFields[0] != "value" || usedFields[1] != "other" {
		t.Fatalf("ReceiverUses fields = %v, want [value other]", usedFields)
	}
	if len(calledMethods) != 1 || calledMethods[0] != "SetValue" {
		t.Fatalf("ReceiverUses methods = %v, want [SetValue]", calledMethods)
	}
}
