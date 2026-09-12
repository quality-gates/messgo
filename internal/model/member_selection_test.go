package model

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"
)

func TestSelectedMemberUsesAreQualifiedByType(t *testing.T) {
	f, err := ParseSource("members.go", []byte(`package sample

type S struct {
	secret int
}

type T struct {
	secret int
}

var _ = T{secret: 1}

func (S) do() {}
func (T) do() {}

func use(t T) {
	t.do()
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	want := map[MemberKey]bool{
		{Type: "T", Name: "secret"}: true,
		{Type: "T", Name: "do"}:     true,
	}
	if !f.MemberSelectedForType("T", "secret") || !f.MemberSelectedForType("T", "do") {
		t.Fatalf("typed selection query does not find T members")
	}
	assertMemberUses(t, f, want)
	if f.MemberSelectedForType("S", "secret") || f.MemberSelectedForType("S", "do") {
		t.Fatalf("typed selection query confused unrelated S members with T members")
	}

	copy := SelectedMemberUses(f)
	delete(copy, MemberKey{Type: "T", Name: "secret"})
	if !f.MemberSelectedForType("T", "secret") {
		t.Fatal("SelectedMemberUses returned the live selection index")
	}

	f.PackageMemberSelections = map[MemberKey]bool{{Type: "S", Name: "secret"}: true}
	if !f.MemberSelectedForType("S", "secret") || f.MemberSelectedForType("T", "secret") {
		t.Fatal("package selection index was not preferred by typed selection query")
	}
}

func TestSelectedMemberUsesTrackPromotedMembers(t *testing.T) {
	f, err := ParseSource("promoted.go", []byte(`package sample

type helper struct {
	value int
}

func (helper) Do() {}

type middle struct {
	helper
}

type unrelated struct {
	other int
}

type thing struct {
	ignored int
	unrelated
	middle
}

type shadowed struct {
	helper
}

func (shadowed) Do() {}

type cycleA struct {
	cycleB
}

type cycleB struct {
	cycleA
}

func use(t thing, s shadowed, c cycleA) {
	t.Do()
	_ = t.value
	s.Do()
	c.Do()
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	want := map[MemberKey]bool{
		{Type: "thing", Name: "Do"}:      true,
		{Type: "thing", Name: "value"}:   true,
		{Type: "thing", Name: "middle"}:  true,
		{Type: "middle", Name: "helper"}: true,
		{Type: "shadowed", Name: "Do"}:   true,
		{Type: "cycleA", Name: "Do"}:     true,
	}
	assertMemberUses(t, f, want)
	if f.MemberSelectedForType("shadowed", "helper") {
		t.Fatal("direct shadowing method incorrectly marked the embedded helper as used")
	}
}

func TestMemberTypeResolverExpressions(t *testing.T) {
	f, err := ParseSource("types.go", []byte(`package sample

type Leaf struct {
	value int
}

type Root struct {
	leaf Leaf
}

func makeLeaf() Leaf { return Leaf{} }
func makePair() (Leaf, Leaf) { return Leaf{}, Leaf{} }
func (Root) child() Leaf { return Leaf{} }
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	collector := newMemberSelectionCollector(f)
	resolver := &collector.types
	types := map[string]string{"leaf": "Leaf", "root": "Root", "generic": "Generic"}

	cases := []struct {
		expr string
		want string
	}{
		{expr: "leaf", want: "Leaf"},
		{expr: "root.leaf", want: "Leaf"},
		{expr: "Leaf{}", want: "Leaf"},
		{expr: "value.(Leaf)", want: "Leaf"},
		{expr: "new(Leaf)", want: "Leaf"},
		{expr: "Leaf(0)", want: "Leaf"},
		{expr: "makeLeaf()", want: "Leaf"},
		{expr: "root.child()", want: "Leaf"},
		{expr: "(leaf)", want: "Leaf"},
		{expr: "*leaf", want: "Leaf"},
		{expr: "&leaf", want: "Leaf"},
		{expr: "generic[0]", want: "Generic"},
		{expr: "generic[A, B]", want: "Generic"},
		{expr: "1", want: ""},
		{expr: "unknown", want: ""},
		{expr: "root.unknown", want: ""},
		{expr: "unknown()", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatalf("ParseExpr: %v", err)
			}
			got := resolver.expressionType(expr, types)
			if got != tc.want {
				t.Fatalf("expressionType(%q) = %q, want %q", tc.expr, got, tc.want)
			}
		})
	}
	if path := resolver.promotedMemberPath("Root", "unknown"); path != nil {
		t.Fatalf("promotedMemberPath(Root, unknown) = %v, want nil", path)
	}
	if _, _, ok := resolver.lookupMember("Missing", "unknown", map[string]bool{}); ok {
		t.Fatal("lookupMember(Missing, unknown) = true, want false")
	}
	visiting := map[string]bool{}
	if _, _, ok := resolver.lookupMember("Root", "unknown", visiting); ok {
		t.Fatal("lookupMember(Root, unknown) = true, want false")
	}
	if len(visiting) != 0 {
		t.Fatalf("lookupMember left visiting types = %v, want empty", visiting)
	}
}

func TestSelectedMemberUsesTrackPackageAndLocalScopes(t *testing.T) {
	f, err := ParseSource("scopes.go", []byte(`package sample

type Leaf struct {
	value int
	other int
}

type Root struct {
	leaf Leaf
}

var packageLeaf Leaf
var packageValue = packageLeaf.value
var packageRoot Root
var packageCall = packageRoot.use(Leaf{})
var packageMap = map[string]Leaf{"leaf": Leaf{value: 1}}
var packageSlice = []Leaf{Leaf{value: 1}}
var packageArray = [1]Leaf{Leaf{value: 1}}
var packageUnkeyed = Leaf{1, 2}

func makeLeaf() Leaf { return Leaf{} }
func makePair() (Leaf, Leaf) { return Leaf{}, Leaf{} }

func (r Root) use(param Leaf, ignored []Leaf) {
	const ignoredConstant = 1
	var explicit Leaf
	var inferred = Leaf{value: 1}
	var first, second = Leaf{}, Leaf{}
	var noValue Leaf
	var _, ignoredVariable = Leaf{}, Leaf{}
	var one, two = makePair()
	var assigned int

	explicit = Leaf{}
	assigned = Leaf{}
	_, explicit = makePair()

	explicit.value
	inferred.value
	first.value
	second.value
	noValue.value
	ignoredVariable.value
	assigned.value
	param.value
	r.leaf.value

	func(inner Leaf, ignoredSlice []Leaf) {
		var nested Leaf
		nested.value
		inner.value
	}(Leaf{})
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	want := map[MemberKey]bool{
		{Type: "Leaf", Name: "value"}: true,
		{Type: "Leaf", Name: "other"}: true,
		{Type: "Root", Name: "leaf"}:  true,
		{Type: "Root", Name: "use"}:   true,
	}
	assertMemberUses(t, f, want)
	if !f.MemberSelectedForType("Leaf", "other") {
		t.Fatal("unkeyed Leaf literal did not select every Leaf field")
	}
}

func TestMemberSelectionCompositeLiteralCases(t *testing.T) {
	f, err := ParseSource("literals.go", []byte(`package sample

type Leaf struct {
	value int
	other int
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	collector := newMemberSelectionCollector(f)
	recorder := &collector.recorder

	tests := []struct {
		expr string
		want map[MemberKey]bool
	}{
		{expr: "map[string]int{field: 1}", want: map[MemberKey]bool{}},
		{expr: "Leaf{value: 1}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
		{expr: "Leaf{1, 2}", want: map[MemberKey]bool{
			{Type: "Leaf", Name: "value"}: true,
			{Type: "Leaf", Name: "other"}: true,
		}},
		{expr: "Leaf{}", want: map[MemberKey]bool{}},
		{expr: "unknown{1}", want: map[MemberKey]bool{}},
		{expr: "external.Type{value: 1}", want: map[MemberKey]bool{{Type: "external.Type", Name: "value"}: true}},
		{expr: "Leaf{1, value: 2}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
		{expr: "Leaf{1: 2}", want: map[MemberKey]bool{}},
	}
	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatalf("ParseExpr: %v", err)
			}
			lit, ok := expr.(*ast.CompositeLit)
			if !ok {
				t.Fatalf("ParseExpr(%q) = %T, want composite literal", tc.expr, expr)
			}
			names := map[string]bool{}
			uses := map[MemberKey]bool{}
			recorder.recordComposite(lit, names, uses)
			if len(uses) != len(tc.want) {
				t.Fatalf("recordComposite(%q) = %v, want %v", tc.expr, uses, tc.want)
			}
			for key := range tc.want {
				if !uses[key] {
					t.Errorf("recordComposite(%q)[%+v] = false, want true", tc.expr, key)
				}
			}
		})
	}
}

func TestMemberSelectionHelpersHandleMissingSyntax(t *testing.T) {
	collector := newMemberSelectionCollector(&File{Syntax: &ast.File{}})
	names := map[string]bool{}
	uses := map[MemberKey]bool{}
	collector.collectFunc(&ast.FuncDecl{Type: &ast.FuncType{}}, nil, names, uses)
	collector.scope.addFuncParameters(&ast.FuncDecl{}, map[string]string{})
	collector.scope.addFuncLiteralParameters(&ast.FuncLit{}, map[string]string{})
	collector.scope.addDeclaration(&ast.DeclStmt{}, map[string]string{})
	collector.scope.addAssignment(&ast.AssignStmt{Lhs: []ast.Expr{&ast.Ident{Name: "x"}}}, map[string]string{})

	file := &File{Syntax: &ast.File{Decls: []ast.Decl{
		&ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.TypeSpec{}}},
	}}}
	if got := collector.packageTypes(file); len(got) != 0 {
		t.Fatalf("packageTypes(invalid var spec) = %v, want empty", got)
	}
}

func TestMemberTypeNameExpressions(t *testing.T) {
	cases := map[string]string{
		"T":             "T",
		"other.T":       "other.T",
		"(T)":           "T",
		"*T":            "T",
		"Generic[int]":  "Generic",
		"Generic[A, B]": "Generic",
		"42":            "",
	}
	for source, want := range cases {
		t.Run(source, func(t *testing.T) {
			expr, err := parser.ParseExpr(source)
			if err != nil {
				t.Fatalf("ParseExpr: %v", err)
			}
			if got := memberTypeName(expr); got != want {
				t.Fatalf("memberTypeName(%q) = %q, want %q", source, got, want)
			}
		})
	}
}

func TestFunctionResultType(t *testing.T) {
	if got := functionResultType(&Function{}); got != "" {
		t.Fatalf("functionResultType(no results) = %q, want empty", got)
	}
	if got := functionResultType(&Function{Results: []*Parameter{{}, {}}}); got != "" {
		t.Fatalf("functionResultType(multiple results) = %q, want empty", got)
	}
	if got := functionResultType(&Function{Results: []*Parameter{{Field: &ast.Field{}}}}); got != "" {
		t.Fatalf("functionResultType(no result type) = %q, want empty", got)
	}
	if got := functionResultType(&Function{Results: []*Parameter{{Field: &ast.Field{Type: &ast.Ident{Name: "Leaf"}}}}}); got != "Leaf" {
		t.Fatalf("functionResultType(Leaf) = %q, want Leaf", got)
	}
}

func TestFunctionResultTypes(t *testing.T) {
	if got := functionResultTypes(nil); got != nil {
		t.Fatalf("functionResultTypes(nil) = %v, want nil", got)
	}
	if got := functionResultTypes(&Function{}); got != nil {
		t.Fatalf("functionResultTypes(empty) = %v, want nil", got)
	}
	fn := &Function{
		Results: []*Parameter{
			{Field: nil},
			{Field: &ast.Field{Type: &ast.StarExpr{X: &ast.Ident{Name: "Worker"}}}},
			{Field: &ast.Field{Type: &ast.Ident{Name: "error"}}},
		},
	}
	got := functionResultTypes(fn)
	if !slices.Equal(got, []string{"", "Worker", "error"}) {
		t.Fatalf("functionResultTypes(multi) = %v, want [\"\" Worker error]", got)
	}
}

func TestUnwrapParen(t *testing.T) {
	ident := &ast.Ident{Name: "x"}
	if got := unwrapParen(ident); got != ident {
		t.Fatalf("unwrapParen(ident) = %v, want %v", got, ident)
	}
	paren1 := &ast.ParenExpr{X: ident}
	paren2 := &ast.ParenExpr{X: paren1}
	if got := unwrapParen(paren2); got != ident {
		t.Fatalf("unwrapParen(paren2) = %v, want %v", got, ident)
	}
}

func TestCallResultTypes(t *testing.T) {
	f, err := ParseSource("calls.go", []byte(`package sample

type Leaf struct{ value int }
type Root struct{ leaf Leaf }
func (Root) Child() Leaf { return Leaf{} }
func (Root) Pair() (Leaf, Leaf) { return Leaf{}, Leaf{} }
type Composite struct{ Root }

type BaseIface interface {
	BaseCall() (Leaf, Leaf)
}

type SubIface interface {
	BaseIface
	SubCall() Leaf
}

func makeLeaf() Leaf { return Leaf{} }
func makePair() (Leaf, Leaf) { return Leaf{}, Leaf{} }
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	collector := newMemberSelectionCollector(f)
	resolver := &collector.types
	types := map[string]string{
		"root": "Root",
		"comp": "Composite",
		"sub":  "SubIface",
	}

	if got := resolver.callResultTypes(nil, types); got != nil {
		t.Fatalf("callResultTypes(nil) = %v, want nil", got)
	}

	cases := []struct {
		expr string
		want []string
	}{
		{expr: "new(Leaf)", want: []string{"Leaf"}},
		{expr: "new()", want: nil},
		{expr: "Leaf(0)", want: []string{"Leaf"}},
		{expr: "makeLeaf()", want: []string{"Leaf"}},
		{expr: "makePair()", want: []string{"Leaf", "Leaf"}},
		{expr: "root.Child()", want: []string{"Leaf"}},
		{expr: "root.Pair()", want: []string{"Leaf", "Leaf"}},
		{expr: "comp.Child()", want: []string{"Leaf"}},
		{expr: "comp.Pair()", want: []string{"Leaf", "Leaf"}},
		{expr: "sub.SubCall()", want: []string{"Leaf"}},
		{expr: "sub.BaseCall()", want: []string{"Leaf", "Leaf"}},
		{expr: "unknown()", want: nil},
		{expr: "root.Unknown()", want: nil},
		{expr: "sub.Unknown()", want: nil},
		{expr: "unknown.Child()", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatalf("ParseExpr: %v", err)
			}
			call, ok := expr.(*ast.CallExpr)
			if !ok {
				t.Fatalf("%s is not a call", tc.expr)
			}
			got := resolver.callResultTypes(call, types)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("callResultTypes(%s) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}

	visiting := map[string]bool{}
	if got := lookupMethod(resolver.classes, resolver.interfaces, "Root", "Unknown", visiting); got != nil {
		t.Fatalf("lookupMethod(Unknown) = %v, want nil", got)
	}
	if len(visiting) != 0 {
		t.Fatalf("visiting map was not cleared: %v", visiting)
	}
	if got := lookupMethod(resolver.classes, resolver.interfaces, "NonExistent", "Child", visiting); got != nil {
		t.Fatalf("lookupMethod(NonExistent) = %v, want nil", got)
	}
	visitingCycle := map[string]bool{"Cycle": true}
	if got := lookupMethod(resolver.classes, resolver.interfaces, "Cycle", "Child", visitingCycle); got != nil {
		t.Fatalf("lookupMethod(visiting cycle) = %v, want nil", got)
	}
}

func TestSelectedMemberUsesMultiReturnCalls(t *testing.T) {
	f, err := ParseSource("multi_return.go", []byte(`package sample

type WorkerA struct { fieldA int }
func (w *WorkerA) WorkA() {}
func newWorkerA() (*WorkerA, error) { return &WorkerA{}, nil }

type WorkerB struct { fieldB int }
func (w *WorkerB) WorkB() {}
func newWorkerB() (error, *WorkerB) { return nil, &WorkerB{} }

type WorkerC struct { fieldC int }
func (w *WorkerC) WorkC() {}
func newWorkerC() (*WorkerC, error) { return &WorkerC{}, nil }

type WorkerD struct { fieldD int }
func (w *WorkerD) WorkD() {}
type FactoryD struct{}
func (FactoryD) CreateD() (*WorkerD, error) { return &WorkerD{}, nil }

type WorkerE struct { fieldE int }
func (w *WorkerE) WorkE() {}
type BaseE struct{}
func (BaseE) CreateE() (*WorkerE, error) { return &WorkerE{}, nil }
type CompositeE struct{ BaseE }

type WorkerF struct { fieldF int }
func (w *WorkerF) WorkF() {}

type WorkerG struct { fieldG int }
func (w *WorkerG) WorkG() {}

func testFunc() {
	wA, errA := newWorkerA()
	_ = errA
	wA.WorkA()
	_ = wA.fieldA

	_, wB := newWorkerB()
	wB.WorkB()
	_ = wB.fieldB

	var wC, errC = newWorkerC()
	_ = errC
	wC.WorkC()
	_ = wC.fieldC

	var f FactoryD
	wD, errD := f.CreateD()
	_ = errD
	wD.WorkD()
	_ = wD.fieldD

	var comp CompositeE
	wE, errE := comp.CreateE()
	_ = errE
	wE.WorkE()
	_ = wE.fieldE

	var wF1, wF2 WorkerF
	wF1.WorkF()
	wF2.WorkF()

	wG1, wG2 := WorkerG{}, WorkerG{}
	wG1.WorkG()
	wG2.WorkG()
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	for _, check := range []struct{ typ, member string }{
		{"WorkerA", "fieldA"}, {"WorkerA", "WorkA"},
		{"WorkerB", "fieldB"}, {"WorkerB", "WorkB"},
		{"WorkerC", "fieldC"}, {"WorkerC", "WorkC"},
		{"WorkerD", "fieldD"}, {"WorkerD", "WorkD"},
		{"WorkerE", "fieldE"}, {"WorkerE", "WorkE"},
		{"WorkerF", "WorkF"},
		{"WorkerG", "WorkG"},
	} {
		if !f.MemberSelectedForType(check.typ, check.member) {
			t.Errorf("%s.%s was not marked as selected", check.typ, check.member)
		}
	}
}

func assertMemberUses(t *testing.T, f *File, want map[MemberKey]bool) {
	t.Helper()
	got := SelectedMemberUses(f)
	if len(got) != len(want) {
		t.Fatalf("SelectedMemberUses() = %v, want %v", got, want)
	}
	for key := range want {
		if !got[key] {
			t.Errorf("SelectedMemberUses()[%+v] = false, want true", key)
		}
	}
}
