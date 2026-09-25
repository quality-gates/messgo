package model

import (
	"go/ast"
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

	if !HasGoto(f.Functions[0]) {
		t.Fatalf("%s HasGoto() = false, want true", f.Functions[0].Name)
	}
	if HasGoto(f.Functions[1]) {
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

	calls := LoopConditionCalls(f.Functions[0], map[string]bool{"len": true, "cap": true})
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

	calls := Calls(f.Functions[0])
	want := []Call{
		{Name: "os.Exit", Selector: "Exit", Line: 4},
		{Name: "syscall.Exit", Selector: "Exit", Line: 5},
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

func TestFunctionCallsResolveImportedPackagePaths(t *testing.T) {
	src := []byte(`package sample

import (
	o "os"
	"syscall"
	os "github.com/acme/os"
)

type target struct{}

func (target) Exit(int) {}

func stop(os target) {
	o.Exit(1)
	syscall.Exit(1)
	os.Exit(1)
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	calls := Calls(f.Functions[0])
	want := []struct {
		name, selector, packagePath string
	}{
		{name: "o.Exit", selector: "Exit", packagePath: "os"},
		{name: "syscall.Exit", selector: "Exit", packagePath: "syscall"},
		{name: "os.Exit", selector: "Exit"},
	}
	if len(calls) != len(want) {
		t.Fatalf("Calls() = %+v, want %d calls", calls, len(want))
	}
	for i, call := range calls {
		if call.Name != want[i].name || call.Selector != want[i].selector || call.PackagePath != want[i].packagePath {
			t.Errorf("Calls()[%d] = %+v, want name %q, selector %q, package path %q", i, call, want[i].name, want[i].selector, want[i].packagePath)
		}
	}
}

func TestCallMetadataHelpers(t *testing.T) {
	qualifier := ast.NewIdent("o")
	selector := &ast.SelectorExpr{X: qualifier, Sel: ast.NewIdent("Exit")}
	if got, ok := calledSelector(&ast.ParenExpr{X: selector}); !ok || got != selector {
		t.Fatalf("calledSelector(parenthesized selector) = %v, %t; want selector, true", got, ok)
	}
	if got, ok := calledSelector(qualifier); ok || got != nil {
		t.Fatalf("calledSelector(identifier) = %v, %t; want nil, false", got, ok)
	}
	if got := packageQualifier(&ast.ParenExpr{X: qualifier}); got != qualifier {
		t.Fatalf("packageQualifier(parenthesized identifier) = %v, want %v", got, qualifier)
	}
	nestedSelector := &ast.SelectorExpr{X: qualifier, Sel: ast.NewIdent("pkg")}
	if got := packageQualifier(nestedSelector); got != nil {
		t.Fatalf("packageQualifier(nested selector) = %v, want nil", got)
	}

	file := &File{Syntax: &ast.File{}}
	bound := ast.NewIdent("o")
	bound.Obj = &ast.Object{}
	for _, tc := range []struct {
		name      string
		file      *File
		qualifier *ast.Ident
		want      bool
	}{
		{name: "nil file", qualifier: qualifier},
		{name: "nil syntax", file: &File{}, qualifier: qualifier},
		{name: "nil qualifier", file: file},
		{name: "bound qualifier", file: file, qualifier: bound},
		{name: "unbound qualifier", file: file, qualifier: qualifier, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolvableImportQualifier(tc.file, tc.qualifier); got != tc.want {
				t.Fatalf("resolvableImportQualifier() = %t, want %t", got, tc.want)
			}
		})
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

	if got := EmptyNilCheckBlockLines(fn); len(got) != 1 || got[0] != 4 {
		t.Fatalf("EmptyNilCheckBlockLines() = %v, want [4]", got)
	}
	if got := ElseBlockLines(fn); len(got) != 1 || got[0] != 5 {
		t.Fatalf("ElseBlockLines() = %v, want [5]", got)
	}
	assigns := IfAssignmentInitPositions(fn)
	if len(assigns) != 1 || assigns[0].Line != 8 || assigns[0].Column != 5 {
		t.Fatalf("IfAssignmentInitPositions() = %+v, want line 8 column 5", assigns)
	}
	dups := DuplicateLiteralKeys(fn)
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

func TestSelectedMemberNamesTracksUnkeyedLocalStructFields(t *testing.T) {
	f, err := ParseSource("query.go", []byte(`package sample
import "image"

type other struct {
	otherField int
}

type key struct {
	typ string
	ptr any
}

type generic[T any] struct {
	first  T
	second T
}

type pair[A, B any] struct {
	left  A
	right B
}

type empty struct {
	field int
}

var _ = key{"", nil}
var _ = generic[int]{1, 2}
var _ = pair[int, string]{1, ""}
var _ = image.Point{1, 2}
var _ = []int{1}
var _ = [1]int{1}
var _ = map[int]int{1: 2}
var _ = empty{}
var _ = unknown{1}
`))
	if err != nil {
		t.Fatal(err)
	}
	selected := f.SelectedMemberNames()
	for _, name := range []string{"typ", "ptr", "first", "second", "left", "right"} {
		if !selected[name] {
			t.Errorf("SelectedMemberNames()[%q] = false, want true", name)
		}
	}
	for _, name := range []string{"otherField", "field"} {
		if selected[name] {
			t.Errorf("SelectedMemberNames()[%q] = true, want false", name)
		}
	}
}

func TestSelectedMemberNamesUsesPackageStructsForUnkeyedLiterals(t *testing.T) {
	use, err := ParseSource("use.go", []byte(`package sample
var _ = key{1, 2}
`))
	if err != nil {
		t.Fatal(err)
	}
	declaration, err := ParseSource("key.go", []byte(`package sample
type key struct {
	first  int
	second int
}
`))
	if err != nil {
		t.Fatal(err)
	}
	use.PackageClasses = declaration.Classes
	selected := use.SelectedMemberNames()
	if !selected["first"] || !selected["second"] {
		t.Fatalf("SelectedMemberNames() = %v, want package-declared key fields", selected)
	}
}

func TestDuplicateLiteralKeysUseConstantValue(t *testing.T) {
	f, err := ParseSource("query.go", []byte("package sample\nfunc f() {\n\t_ = map[int]int{1: 0, 01: 0}\n\t_ = map[string]int{\"a\": 0, `a`: 0}\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := DuplicateLiteralKeys(f.Functions[0]); len(got) < 2 {
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
	if got := EmptyNilCheckBlockLines(f.Functions[0]); len(got) != 0 {
		t.Fatalf("EmptyNilCheckBlockLines() = %v, want none for == nil", got)
	}
}
