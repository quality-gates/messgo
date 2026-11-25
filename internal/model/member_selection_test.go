package model

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
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
	f.PackageMemberSelections = map[MemberKey]bool{}
	if f.MemberSelectedForType("T", "secret") {
		t.Fatal("empty package selections should override the file cache")
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
		{Type: "helper", Name: "Do"}:     true,
		{Type: "helper", Name: "value"}:  true,
		{Type: "shadowed", Name: "Do"}:   true,
		{Type: "cycleA", Name: "Do"}:     true,
	}
	assertMemberUses(t, f, want)
	if f.MemberSelectedForType("shadowed", "helper") {
		t.Fatal("direct shadowing method incorrectly marked the embedded helper as used")
	}
}

func TestSelectedMemberUsesForNonStructTypeExpressions(t *testing.T) {
	f, err := ParseSource("types.go", []byte(`package sample

type StatusCode int
func use(code StatusCode) {
	code.valueMethod()
	StatusCode(1).conversionMethod()
	StatusCode.methodExpression(StatusCode(1))
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	assertMemberUses(t, f, map[MemberKey]bool{
		{Type: "StatusCode", Name: "valueMethod"}:      true,
		{Type: "StatusCode", Name: "conversionMethod"}: true,
		{Type: "StatusCode", Name: "methodExpression"}: true,
	})
}

func TestPackageTypeDefsResolveNonStructMethodExpressions(t *testing.T) {
	f, err := ParseSource("uses.go", []byte(`package sample
func use() {
	StatusCode(1).conversionMethod()
	StatusCode.methodExpression(StatusCode(1))
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	f.PackageTypeDefs = map[string]ast.Expr{"StatusCode": ast.NewIdent("int")}

	assertMemberUses(t, f, map[MemberKey]bool{
		{Type: "StatusCode", Name: "conversionMethod"}: true,
		{Type: "StatusCode", Name: "methodExpression"}: true,
	})
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
	types := map[string]memberVarType{"leaf": {name: "Leaf"}, "root": {name: "Root"}, "generic": {name: "Generic"}}

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
	if path := resolver.promotedMemberPathTyped(memberVarType{name: "Root"}, "unknown"); path != nil {
		t.Fatalf("promotedMemberPath(Root, unknown) = %v, want nil", path)
	}
	if _, _, ok := resolver.lookupMemberTyped(memberVarType{name: "Missing"}, "unknown", map[string]bool{}); ok {
		t.Fatal("lookupMember(Missing, unknown) = true, want false")
	}
	visited := map[string]bool{}
	if _, _, ok := resolver.lookupMemberTyped(memberVarType{name: "Root"}, "unknown", visited); ok {
		t.Fatal("lookupMember(Root, unknown) = true, want false")
	}
	if len(visited) != 1 || !visited["Root"] {
		t.Fatalf("lookupMember visited types = %v, want Root", visited)
	}
}

func TestDiamondMemberAndMethodLookupVisitsEachTypeOnce(t *testing.T) {
	const levels = 24

	f, err := ParseSource("diamond.go", []byte(diamondLookupSource(levels)))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	collector := newMemberSelectionCollector(f)
	root := fmt.Sprintf("F%d", levels)

	memberVisited := map[string]bool{}
	if _, path, ok := collector.types.lookupMemberTyped(memberVarType{name: root}, "Missing", memberVisited); ok || path != nil {
		t.Fatalf("lookupMember(%s, Missing) = (path %v, ok %v), want a failed lookup", root, path, ok)
	}
	if len(memberVisited) != levels+1 {
		t.Fatalf("lookupMember visited %d types, want %d: %v", len(memberVisited), levels+1, memberVisited)
	}

	methodVisited := map[string]bool{}
	if got := lookupMethod(collector.types.classes, collector.types.interfaces, root, "Missing", methodVisited); got != nil {
		t.Fatalf("lookupMethod(%s, Missing) = %v, want nil", root, got)
	}
	if len(methodVisited) != levels+1 {
		t.Fatalf("lookupMethod visited %d types, want %d: %v", len(methodVisited), levels+1, methodVisited)
	}

	visited := map[string]bool{}
	if _, path, ok := collector.types.lookupMemberTyped(memberVarType{name: root}, "Z", visited); ok || path != nil {
		t.Fatalf("lookupMember(%s, Z) = (path %v, ok %v), want an ambiguous lookup", root, path, ok)
	}
	if len(visited) != levels+1 {
		t.Fatalf("ambiguous lookup visited %d types, want %d: %v", len(visited), levels+1, visited)
	}
}

func diamondLookupSource(levels int) string {
	var source strings.Builder
	source.WriteString("package sample\n\ntype F0 struct { Z int }\n")
	for level := 1; level <= levels; level++ {
		fmt.Fprintf(&source, "type F%d struct { F%d; F%d }\n", level, level-1, level-1)
	}
	return source.String()
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

type Other struct {
	otherVal int
}

type Leaves []Leaf
type LeafMap map[string]Leaf
type Chained Leaves
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
		{expr: "Leaf{1: 2}", want: map[MemberKey]bool{}},
		{expr: "[]Leaf{{value: 1}}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
		{expr: "[]Leaf{Other{otherVal: 1}}", want: map[MemberKey]bool{}},
		{expr: "[]Leaf{{1, 2}}", want: map[MemberKey]bool{
			{Type: "Leaf", Name: "value"}: true,
			{Type: "Leaf", Name: "other"}: true,
		}},
		{expr: "[]*Leaf{{value: 1}}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
		{expr: "map[string]Leaf{\"k\": {value: 1}}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
		{expr: "map[Leaf]string{{value: 1}: \"v\"}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
		{expr: "map[string]*Leaf{\"k\": {value: 1}}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
		{expr: "map[*Leaf]string{{value: 1}: \"v\"}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
		{expr: "[][]Leaf{{{value: 1}}}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
		{expr: "[2]Leaf{{value: 1}}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
		{expr: "[2]Leaf{1: {value: 1}}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
		{expr: "[]Leaf{{}}", want: map[MemberKey]bool{}},
		{expr: "Leaves{{value: 1}}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
		{expr: "LeafMap{\"k\": {value: 1}}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
		{expr: "Chained{{value: 1}}", want: map[MemberKey]bool{{Type: "Leaf", Name: "value"}: true}},
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
			uses := map[MemberKey]bool{}
			recorder.recordComposite(lit, uses)
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

func TestCollectPackageTypeDefsHandlesEdgeCases(t *testing.T) {
	emptyFile := &File{}
	defs := collectPackageTypeDefs(emptyFile, nil)
	if len(defs) != 0 {
		t.Fatalf("expected empty defs, got %v", defs)
	}

	dummyClass := &Class{Name: "Dummy"}
	defs = collectPackageTypeDefs(emptyFile, []*Class{dummyClass, {File: nil}})
	if len(defs) != 0 {
		t.Fatalf("expected empty defs, got %v", defs)
	}

	f, err := ParseSource("sample.go", []byte(`package sample
const ConstVal = 123
type A struct{}
type B struct{}
type S []int
`))
	if err != nil {
		t.Fatal(err)
	}
	// Test collectFromFile(f.Syntax) when classes is nil
	defsSingle := collectPackageTypeDefs(f, nil)
	if defsSingle["S"] == nil {
		t.Fatal("expected type S to be defined from f directly")
	}

	// Test collectFromFile(class.File.Syntax) when f has no S but other class file does
	emptyF, err := ParseSource("empty.go", []byte(`package sample`))
	if err != nil {
		t.Fatal(err)
	}
	classes := []*Class{
		{File: f},
	}
	defsFromClass := collectPackageTypeDefs(emptyF, classes)
	if defsFromClass["S"] == nil {
		t.Fatal("expected type S to be defined from class file")
	}

	// Test collectFromFile handles nil syntax
	collectFromFile(nil, defs)

	// Test collectFromFile ignores nil TypeSpec.Type
	nilTypeDefs := make(map[string]ast.Expr)
	collectFromFile(&ast.File{
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.TYPE,
				Specs: []ast.Spec{
					&ast.TypeSpec{Name: ast.NewIdent("NilType"), Type: nil},
				},
			},
		},
	}, nilTypeDefs)
	if nilTypeDefs["NilType"] != nil {
		t.Fatal("expected NilType with nil Type not to be recorded")
	}
}

func TestUnderlyingTypeEdgeCases(t *testing.T) {
	loopIdent := ast.NewIdent("Loop")
	recorder := &memberUseRecorder{
		resolver: &memberTypeResolver{
			typeDefs: map[string]ast.Expr{
				"Loop": loopIdent,
			},
		},
	}
	if got := recorder.underlyingType(nil); got != nil {
		t.Fatalf("underlyingType(nil) = %v, want nil", got)
	}
	if got := recorder.underlyingType(loopIdent); got != loopIdent {
		t.Fatalf("underlyingType(Loop) = %v, want self", got)
	}
}

func TestMemberSelectionHelpersHandleMissingSyntax(t *testing.T) {
	collector := newMemberSelectionCollector(&File{Syntax: &ast.File{}})
	uses := map[MemberKey]bool{}
	collector.collectFunc(&ast.FuncDecl{Type: &ast.FuncType{}}, nil, uses)
	collector.scope.addFuncParameters(&ast.FuncDecl{}, map[string]memberVarType{})
	collector.scope.addFuncLiteralParameters(&ast.FuncLit{}, map[string]memberVarType{})
	collector.scope.addDeclaration(&ast.DeclStmt{}, map[string]memberVarType{})
	collector.scope.addAssignment(&ast.AssignStmt{Lhs: []ast.Expr{&ast.Ident{Name: "x"}}}, map[string]memberVarType{})

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
		"[]T":           "T",
		"[]*T":          "T",
		"[3]T":          "T",
		"chan T":        "T",
		"chan *T":       "T",
		"map[string]T":  "T",
		"map[string]*T": "T",
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

func TestMemberVarTypeOfContainerShapes(t *testing.T) {
	cases := []struct {
		source string
		want   memberVarType
	}{
		{source: "T", want: memberVarType{name: "T"}},
		{source: "[]T", want: memberVarType{name: "T", kind: containerSequence}},
		{source: "[3]T", want: memberVarType{name: "T", kind: containerSequence}},
		{source: "*[]T", want: memberVarType{name: "T", kind: containerSequence}},
		{source: "([]T)", want: memberVarType{name: "T", kind: containerSequence}},
		{source: "chan T", want: memberVarType{name: "T", kind: containerChannel}},
		{source: "map[K]V", want: memberVarType{name: "V", key: "K", kind: containerMap}},
		{source: "map[*K]*V", want: memberVarType{name: "V", key: "K", kind: containerMap}},
		{source: "map[K]struct{}", want: memberVarType{key: "K", kind: containerMap}},
		{source: "42", want: memberVarType{}},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.source)
			if err != nil {
				t.Fatalf("ParseExpr: %v", err)
			}
			if got := memberVarTypeOf(expr); !sameMemberVarType(got, tc.want) {
				t.Fatalf("memberVarTypeOf(%q) = %+v, want %+v", tc.source, got, tc.want)
			}
		})
	}
	if got := memberVarTypeOf(&ast.MapType{Key: &ast.Ident{Name: "K"}, Value: &ast.Ident{Name: "V"}}); got.empty() {
		t.Fatal("memberVarTypeOf(map) reported an empty type")
	}
	if !(memberVarType{}).empty() {
		t.Fatal("zero memberVarType is not empty")
	}
}

func TestFunctionResultType(t *testing.T) {
	if got := functionResultType(&Function{}).name; got != "" {
		t.Fatalf("functionResultType(no results) = %q, want empty", got)
	}
	if got := functionResultType(&Function{Results: []*Parameter{{}, {}}}).name; got != "" {
		t.Fatalf("functionResultType(multiple results) = %q, want empty", got)
	}
	if got := functionResultType(&Function{Results: []*Parameter{{Field: &ast.Field{}}}}).name; got != "" {
		t.Fatalf("functionResultType(no result type) = %q, want empty", got)
	}
	if got := functionResultType(&Function{Results: []*Parameter{{Field: &ast.Field{Type: &ast.Ident{Name: "Leaf"}}}}}).name; got != "Leaf" {
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
	got := memberTypeNames(functionResultTypes(fn))
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
	types := map[string]memberVarType{
		"root": {name: "Root"},
		"comp": {name: "Composite"},
		"sub":  {name: "SubIface"},
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
			got := memberTypeNames(resolver.callResultTypes(call, types))
			if !slices.Equal(got, tc.want) {
				t.Fatalf("callResultTypes(%s) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}

	visited := map[string]bool{}
	if got := lookupMethod(resolver.classes, resolver.interfaces, "Root", "Unknown", visited); got != nil {
		t.Fatalf("lookupMethod(Unknown) = %v, want nil", got)
	}
	if len(visited) != 1 || !visited["Root"] {
		t.Fatalf("lookupMethod visited types = %v, want Root", visited)
	}
	if got := lookupMethod(resolver.classes, resolver.interfaces, "NonExistent", "Child", visited); got != nil {
		t.Fatalf("lookupMethod(NonExistent) = %v, want nil", got)
	}
	visitedCycle := map[string]bool{"Cycle": true}
	if got := lookupMethod(resolver.classes, resolver.interfaces, "Cycle", "Child", visitedCycle); got != nil {
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

func TestSelectedMemberUsesPackageFunctions(t *testing.T) {
	defFile, err := ParseSource("def.go", []byte(`package sample

type Worker struct {
	field int
}

func (w *Worker) Work() {}

func newWorker() *Worker {
	return &Worker{}
}
`))
	if err != nil {
		t.Fatalf("ParseSource def.go: %v", err)
	}

	useFile, err := ParseSource("use.go", []byte(`package sample

func Run() {
	w := newWorker()
	w.Work()
	_ = w.field
}
`))
	if err != nil {
		t.Fatalf("ParseSource use.go: %v", err)
	}

	useFile.PackageFunctions = defFile.Functions
	uses := SelectedMemberUses(useFile)

	if !uses[MemberKey{Type: "Worker", Name: "Work"}] {
		t.Errorf("Worker.Work was not marked as used via PackageFunctions")
	}
	if !uses[MemberKey{Type: "Worker", Name: "field"}] {
		t.Errorf("Worker.field was not marked as used via PackageFunctions")
	}
}

func memberTypeNames(types []memberVarType) []string {
	if types == nil {
		return nil
	}
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = t.name
	}
	return names
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

func TestSelectedMemberUsesRangeLoops(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want map[MemberKey]bool
	}{
		{
			name: "slice parameter value iteration",
			src: `package p
type Worker struct { field int }
func (w *Worker) Work() {}
func Run(workers []*Worker) {
	for _, w := range workers {
		w.Work()
		_ = w.field
	}
}`,
			want: map[MemberKey]bool{
				{Type: "Worker", Name: "Work"}:  true,
				{Type: "Worker", Name: "field"}: true,
			},
		},
		{
			name: "map iteration value",
			src: `package p
type Val struct { vField int }
func (v *Val) ValWork() {}
func Run(m map[string]*Val) {
	for _, v := range m {
		v.ValWork()
		_ = v.vField
	}
}`,
			want: map[MemberKey]bool{
				{Type: "Val", Name: "ValWork"}: true,
				{Type: "Val", Name: "vField"}:  true,
			},
		},
		{
			name: "map key iteration",
			src: `package p
type Key struct { kField int }
func (k Key) KWork() {}
type Val struct { vField int }
func (v Val) VWork() {}
func Run(m map[Key]Val) {
	for k := range m {
		k.KWork()
		_ = k.kField
	}
}`,
			want: map[MemberKey]bool{
				{Type: "Key", Name: "KWork"}:  true,
				{Type: "Key", Name: "kField"}: true,
			},
		},
		{
			name: "map key and value iteration",
			src: `package p
type Key struct { kField int }
func (k Key) KWork() {}
type Val struct { vField int }
func (v Val) VWork() {}
func Run(m map[Key]Val) {
	for k, v := range m {
		k.KWork()
		_ = k.kField
		v.VWork()
		_ = v.vField
	}
}`,
			want: map[MemberKey]bool{
				{Type: "Key", Name: "KWork"}:  true,
				{Type: "Key", Name: "kField"}: true,
				{Type: "Val", Name: "VWork"}:  true,
				{Type: "Val", Name: "vField"}: true,
			},
		},
		{
			name: "channel iteration",
			src: `package p
type Worker struct { field int }
func (w *Worker) Work() {}
func Run(ch chan *Worker) {
	for w := range ch {
		w.Work()
		_ = w.field
	}
}`,
			want: map[MemberKey]bool{
				{Type: "Worker", Name: "Work"}:  true,
				{Type: "Worker", Name: "field"}: true,
			},
		},
		{
			name: "slice expression range",
			src: `package p
type Worker struct { field int }
func (w *Worker) Work() {}
func Run(workers []*Worker) {
	for _, w := range workers[1:] {
		w.Work()
	}
}`,
			want: map[MemberKey]bool{
				{Type: "Worker", Name: "Work"}: true,
			},
		},
		{
			name: "range over an indexed container",
			src: `package p
type Worker struct { field int }
func (w *Worker) Work() {}
func Run(groups [][]*Worker) {
	for _, w := range groups[0] {
		w.Work()
	}
}`,
			want: map[MemberKey]bool{
				{Type: "Worker", Name: "Work"}: true,
			},
		},
		{
			name: "map with a memberless key type still binds the value",
			src: `package p
type Key struct { kField int }
func (k Key) KWork() {}
func Run(m map[Key]struct{}) {
	for k := range m {
		k.KWork()
		_ = k.kField
	}
}`,
			want: map[MemberKey]bool{
				{Type: "Key", Name: "KWork"}:  true,
				{Type: "Key", Name: "kField"}: true,
			},
		},
		{
			name: "range key of unresolved type does not clobber an outer binding",
			src: `package p
type Worker struct{}
func (w Worker) Work() {}
func Run(k Worker, m map[struct{}]int) {
	for k := range m {
		_ = k
	}
	k.Work()
}`,
			want: map[MemberKey]bool{
				{Type: "Worker", Name: "Work"}: true,
			},
		},
		{
			name: "range without iteration variables",
			src: `package p
type Worker struct{}
func Run(workers []*Worker) {
	for range workers {}
}`,
			want: map[MemberKey]bool{},
		},
		{
			name: "range with blank iteration variables",
			src: `package p
type Worker struct{}
func Run(workers []*Worker, ch chan *Worker) {
	for _, _ = range workers {}
	for _ = range ch {}
}`,
			want: map[MemberKey]bool{},
		},
		{
			name: "range over untyped identifier",
			src: `package p
type Worker struct{}
func (w *Worker) Work() {}
func Run() {
	var workers any
	for _, w := range workers.([]*Worker) {
		w.Work()
	}
}`,
			want: map[MemberKey]bool{
				{Type: "Worker", Name: "Work"}: true,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseSource("test.go", []byte(tt.src))
			if err != nil {
				t.Fatalf("ParseSource: %v", err)
			}
			assertMemberUses(t, f, tt.want)
		})
	}
}

func TestAddRangeSkipsBlankIdentifier(t *testing.T) {
	collector := newMemberSelectionCollector(&File{Syntax: &ast.File{}})
	types := map[string]memberVarType{
		"workers": {name: "Worker", kind: containerSequence},
	}
	stmt := &ast.RangeStmt{
		Value: &ast.Ident{Name: "_"},
		X:     &ast.Ident{Name: "workers"},
	}
	collector.scope.addRange(stmt, types)
	if _, ok := types["_"]; ok {
		t.Fatalf("addRange recorded blank identifier in types map")
	}
}

func TestAddRangeBindsIterationVariablesByContainerShape(t *testing.T) {
	src := `package p
type Key struct{}
type Val struct{}
func Run(m map[Key]Val, workers []Val, ch chan Val, n int) {
	for k, v := range m {
		_, _ = k, v
	}
	for i, w := range workers {
		_, _ = i, w
	}
	for c := range ch {
		_ = c
	}
	for u := range n {
		_ = u
	}
}`
	f, err := ParseSource("shapes.go", []byte(src))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	collector := newMemberSelectionCollector(f)
	decl, ok := f.Syntax.Decls[len(f.Syntax.Decls)-1].(*ast.FuncDecl)
	if !ok {
		t.Fatalf("last declaration is not a function")
	}
	types := map[string]memberVarType{}
	collector.scope.addFuncParameters(decl, types)
	collector.scope.collectLocalTypes(decl.Body, types)

	want := map[string]string{"k": "Key", "v": "Val", "w": "Val", "c": "Val", "u": "int"}
	for name, typeName := range want {
		if got := types[name].name; got != typeName {
			t.Errorf("types[%q] = %q, want %q", name, got, typeName)
		}
	}
	if got, ok := types["i"]; ok {
		t.Errorf("types[\"i\"] = %+v, want no binding for a slice index variable", got)
	}
}

func TestSelectedMemberUsesBindTypeSwitchVariable(t *testing.T) {
	f, err := ParseSource("typeswitch.go", []byte(`package sample

type parser struct{ timeout int }

func (p *parser) parse() {}

type other struct{ timeout int }

func run(val any) int {
	switch v := val.(type) {
	case *parser:
		v.parse()
		return v.timeout
	case other, *other:
		return v.timeout
	}
	return 0
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	for _, key := range []MemberKey{{Type: "parser", Name: "parse"}, {Type: "parser", Name: "timeout"}} {
		if !f.MemberSelectedForType(key.Type, key.Name) {
			t.Errorf("type switch use of %v not recorded", key)
		}
	}
	if f.MemberSelectedForType("other", "timeout") {
		t.Error("multi-type clause must not bind the switch variable")
	}
}

func TestSelectedMemberUsesTypeSwitchScoping(t *testing.T) {
	f, err := ParseSource("typeswitch_scope.go", []byte(`package sample

type a struct{ one int }
type b struct{ two int }
type c struct{ three int }
type d struct{ four int }
type e struct{ five int }
type g struct{ six int }
type h struct{ seven int }

func run(val any, v a, p g, q h) {
	switch val.(type) {
	case b:
		_ = v.one
	}
	switch w := val.(type) {
	case b:
		func() { _ = w.two }()
	default:
		_ = v.one
	}
	switch _ = (c{three: 1}); y := q.seven.(type) {
	case d:
		_ = y.four
	}
	switch v := val.(type) {
	case nil:
		_ = v.five
	}
	switch p := val.(type) {
	default:
		_ = p.six
	}
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	for _, key := range []MemberKey{{"a", "one"}, {"b", "two"}, {"c", "three"}, {"d", "four"}, {"h", "seven"}} {
		if !f.MemberSelectedForType(key.Type, key.Name) {
			t.Errorf("use of %v not recorded", key)
		}
	}
	if f.MemberSelectedForType("e", "five") {
		t.Error("nil clause must not bind the switch variable")
	}
	if f.MemberSelectedForType("g", "six") {
		t.Error("default clause must shadow the outer variable of the same name")
	}
}

func TestSelectedMemberUsesThroughGenerics(t *testing.T) {
	f, err := ParseSource("generics.go", []byte(`package sample

type Ttt struct {
	data int
}

func (t Ttt) helper() {}

func Id[T any](v T) T { return v }

func First[T any](s []T) T { return s[0] }

type Box[T any] struct {
	V T
}

func (b Box[T]) Get() T { return b.V }

type Pair[A, B any] struct {
	First  A
	Second B
}

func Run(t Ttt, b Box[Ttt], p Pair[Ttt, int], ts []Ttt) {
	Id(t).helper()
	println(Id(t).data)
	println(b.V.data)
	b.Get().helper()
	println(b.Get().data)
	println(Id(b).V.data)
	println(First(ts).data)
	println(p.Second)
	println(Id(p).First.data)
	_ = Box[Ttt]{V: t}
	_ = Id[Ttt](t)
	_ = Id(ts)
	_ = Id(map[string]Ttt{})
	x := Id(ts)
	println(x[0].data)
	m := Id(map[string]Ttt{})
	println(m["k"].data)
}

func RunContainers(cs chan Ttt, out chan Ttt) {
	println(Id(cs))
	for v := range Id(cs) {
		v.helper()
		out <- v
	}
}

type Other struct {
	data int
}

func Unused() {
	_ = Other{}
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	for _, key := range []MemberKey{
		{"Ttt", "helper"},
		{"Ttt", "data"},
		{"Box", "V"},
		{"Box", "Get"},
		{"Pair", "First"},
		{"Pair", "Second"},
	} {
		if !f.MemberSelectedForType(key.Type, key.Name) {
			t.Errorf("use of %v through a generic instantiation not recorded", key)
		}
	}
	if f.MemberSelectedForType("Other", "data") {
		t.Error("generic instantiation credited an unrelated type with the same member name")
	}
}

func TestSelectedMemberUsesThroughGenericShapesIsolated(t *testing.T) {
	preamble := "package sample\ntype Ttt struct { data int }\nfunc (t Ttt) helper() {}\nfunc Id[T any](v T) T { return v }\ntype Box[T any] struct { V T }\nfunc (b Box[T]) Get() T { return b.V }\ntype Pair[A, B any] struct { First A; Second B }\n"
	cases := []struct {
		name, body  string
		typ, member string
		want        bool
	}{
		{"generic call method", "func Run(t Ttt) { Id(t).helper() }\n", "Ttt", "helper", true},
		{"generic call field", "func Run(t Ttt) { println(Id(t).data) }\n", "Ttt", "data", true},
		{"explicit call field", "func Run(t Ttt) { println(Id[Ttt](t).data) }\n", "Ttt", "data", true},
		{"generic struct field", "func Run(b Box[Ttt]) { println(b.V.data) }\n", "Ttt", "data", true},
		{"generic struct field itself", "func Run(b Box[Ttt]) { println(b.V.data) }\n", "Box", "V", true},
		{"generic method result field", "func Run(b Box[Ttt]) { println(b.Get().data) }\n", "Ttt", "data", true},
		{"generic method itself", "func Run(b Box[Ttt]) { b.Get().helper() }\n", "Box", "Get", true},
		{"struct through generic call", "func Run(b Box[Ttt]) { println(Id(b).V.data) }\n", "Ttt", "data", true},
		{"pair first field", "func Run(p Pair[Ttt, int]) { println(p.First.data) }\n", "Ttt", "data", true},
		{"pair second field", "func Run(p Pair[Ttt, int]) { println(p.Second) }\n", "Pair", "Second", true},
		{"generic factory", "func Make[T any](v T) Box[T] { return Box[T]{V: v} }\nfunc Run(t Ttt) { println(Make(t).V.data) }\n", "Ttt", "data", true},
		{"explicit index list", "func Swap[A, B any](a A, b B) Pair[A, B] { return Pair[A, B]{First: a, Second: b} }\nfunc Run(t Ttt) { println(Swap[Ttt, int](t, 1).First.data) }\n", "Ttt", "data", true},
		{"inferred pair factory", "func Swap[A, B any](a A, b B) Pair[A, B] { return Pair[A, B]{First: a, Second: b} }\nfunc Run(t Ttt) { println(Swap(t, 1).First.data) }\n", "Ttt", "data", true},
		{"slice param", "func First[T any](s []T) T { return s[0] }\nfunc Run(ts []Ttt) { println(First(ts).data) }\n", "Ttt", "data", true},
		{"map param pattern", "func Lookup[K comparable, V any](m map[K]V) V { var v V; return v }\nfunc Run(m map[string]Ttt) { println(Lookup(m).data) }\n", "Ttt", "data", true},
		{"chan param", "func Recv[T any](ch chan T) T { var v T; return v }\nfunc Run(ch chan Ttt) { println(Recv(ch).data) }\n", "Ttt", "data", true},
		{"star param", "func Deref[T any](p *T) T { return *p }\nfunc Run(t Ttt) { println(Deref(&t).data) }\n", "Ttt", "data", true},
		{"nested generic param", "func Wrap[T any](b Box[T]) T { return b.V }\nfunc Run(b Box[Ttt]) { println(Wrap(b).data) }\n", "Ttt", "data", true},
		{"variadic param", "func Collect[T any](vs ...T) T { return vs[0] }\nfunc Run(t Ttt) { println(Collect(t).data) }\n", "Ttt", "data", true},
		{"variadic slice", "func Collect[T any](vs ...T) T { return vs[0] }\nfunc Run(ts []Ttt) { println(Collect(ts...).data) }\n", "Ttt", "data", true},
		{"slice through generic call", "func Run(ts []Ttt) { x := Id(ts); println(x[0].data) }\n", "Ttt", "data", true},
		{"map through generic call", "func Run() { m := Id(map[string]Ttt{}); println(m[\"k\"].data) }\n", "Ttt", "data", true},
		{"chan through generic call", "func Run(cs chan Ttt) { for v := range Id(cs) { v.helper() } }\n", "Ttt", "helper", true},
		{"embedded generic", "type Outer struct { Box[Ttt] }\nfunc Run(o Outer) { println(o.V.data) }\n", "Ttt", "data", true},
		{"embedded generic method", "type Outer struct { Box[Ttt] }\nfunc Run(o Outer) { println(o.Get().data) }\n", "Ttt", "data", true},
		{"generic interface", "type Getter[T any] interface { Get() T }\nfunc Run(g Getter[Ttt]) { println(g.Get().data) }\n", "Ttt", "data", true},
		{"conflict drops binding", "func Pick[T any](a T, b T) T { return a }\ntype Other struct { data int }\nfunc Run(t Ttt, o Other) { println(Pick(t, o).data) }\n", "Ttt", "data", false},
		{"unrelated not credited", "type Other struct { data int }\nfunc Run(o Other) { println(Id(o).data) }\n", "Ttt", "data", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := ParseSource("shape.go", []byte(preamble+tc.body))
			if err != nil {
				t.Fatalf("ParseSource: %v", err)
			}
			if got := f.MemberSelectedForType(tc.typ, tc.member); got != tc.want {
				t.Errorf("MemberSelectedForType(%q,%q) = %v, want %v", tc.typ, tc.member, got, tc.want)
			}
		})
	}
}

func TestGenericCallFun(t *testing.T) {

	cases := []struct {
		expr, wantArgs string
		wantFun        bool
	}{
		{"Id", "", true},
		{"Id[Ttt]", "Ttt", true},
		{"pkg.Id[Ttt]", "Ttt", true},
		{"Swap[Ttt, int]", "Ttt;int", true},
		{"a[0]", "", true},
		{"f()", "", false},
	}
	for _, tc := range cases {
		expr, err := parser.ParseExpr(tc.expr)
		if err != nil {
			t.Fatal(err)
		}
		fun, args := genericCallFun(expr)
		if args != tc.wantArgs {
			t.Errorf("genericCallFun(%q) args = %q, want %q", tc.expr, args, tc.wantArgs)
		}
		_, isIdent := fun.(*ast.Ident)
		_, isSel := fun.(*ast.SelectorExpr)
		if (isIdent || isSel) != tc.wantFun {
			t.Errorf("genericCallFun(%q) fun = %T, want ident/selector = %v", tc.expr, fun, tc.wantFun)
		}
	}
}

func TestMemberVarTypeOfSubst(t *testing.T) {

	subst := typeBinding{names: []string{"T", "K"}, args: "Ttt;string"}
	cases := []struct {
		expr, want string
	}{
		{"T", "Ttt"},
		{"K", "string"},
		{"Other", "Other"},
		{"pkg.T", "pkg.T"},
		{"(T)", "Ttt"},
		{"*T", "Ttt"},
		{"Box[T]", "Box[Ttt]"},
		{"Pair[T, K]", "Pair[Ttt,string]"},
		{"[]T", "[]Ttt"},
		{"chan T", "chan Ttt"},
		{"map[K]T", "map[string]Ttt"},
		{"func()", ""},
		{"interface{}", ""},
	}
	for _, tc := range cases {
		var expr ast.Expr
		if tc.expr == "...T" {
			expr = &ast.Ellipsis{Elt: ast.NewIdent("T")}
		} else {
			var err error
			expr, err = parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatal(err)
			}
		}
		if got := memberVarTypeOfSubst(expr, subst).canonical(); got != tc.want {
			t.Errorf("memberVarTypeOfSubst(%q) = %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestInferredTypeArg(t *testing.T) {

	parse := func(expr string) ast.Expr {
		e, err := parser.ParseExpr(expr)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	cases := []struct {
		pattern, actual string
		want            string
		conflict        bool
	}{
		{"T", "Ttt", "Ttt", false},
		{"Other", "Ttt", "", false},
		{"*T", "Ttt", "Ttt", false},
		{"Box[T]", "Box[Ttt]", "Ttt", false},
		{"Box[T]", "Other", "", false},
		{"Pair[T, int]", "Pair[Ttt, int]", "Ttt", false},
		{"Pair[T, T]", "Pair[Ttt, int]", "", true},
		{"[]T", "[]Ttt", "Ttt", false},
		{"[]T", "Ttt", "", false},
		{"...T", "[]Ttt", "Ttt", false},
		{"...T", "Ttt", "Ttt", false},
		{"chan T", "chan Ttt", "Ttt", false},
		{"chan T", "Ttt", "", false},
		{"map[string]T", "map[string]Ttt", "Ttt", false},
		{"map[T]int", "map[string]int", "string", false},
		{"map[T]T", "map[int]int", "int", false},
		{"map[T]T", "map[int]string", "", true},
		{"map[string]T", "Other", "", false},
	}
	for _, tc := range cases {
		var pattern ast.Expr
		if tc.pattern == "...T" {
			pattern = &ast.Ellipsis{Elt: ast.NewIdent("T")}
		} else {
			pattern = parse(tc.pattern)
		}
		got, conflict := inferredTypeArg(pattern, memberVarTypeOf(parse(tc.actual)), "T")
		if got != tc.want || conflict != tc.conflict {
			t.Errorf("inferredTypeArg(%q,%q) = (%q,%v), want (%q,%v)", tc.pattern, tc.actual, got, conflict, tc.want, tc.conflict)
		}
	}
}

func TestGenericTypeEncoding(t *testing.T) {

	for _, typ := range []memberVarType{
		{name: "Ttt"},
		{name: "Box", args: "Ttt"},
		{name: "Ttt", kind: containerSequence},
		{name: "Ttt", kind: containerChannel},
		{name: "Ttt", kind: containerMap, key: "string"},
		{name: "Pair", args: "Ttt;int"},
	} {
		canon := typ.canonical()
		back := memberVarTypeFromString(canon)
		if !sameMemberVarType(back, typ) {
			t.Errorf("roundtrip %q: got %+v, want %+v", canon, back, typ)
		}
	}
	if got := memberVarTypeFromString(""); !got.empty() {
		t.Errorf("empty string gave %+v", got)
	}
	if got := memberVarTypeFromString("!!!"); !got.empty() {
		t.Errorf("invalid type gave %+v", got)
	}

}

func TestGenericConversionAndBinding(t *testing.T) {
	id := ast.NewIdent("MyType")
	if got := conversionType(id, "Ttt"); got.name != "MyType" || got.args != "Ttt" {
		t.Errorf("conversionType with args = %+v", got)
	}
	if got := conversionType(id, ""); got.name != "MyType" || got.args != "" {
		t.Errorf("conversionType without args = %+v", got)
	}

	classes := map[string]*Class{"Box": {Name: "Box", TypeParams: []string{"T"}}}
	ifaces := map[string]*Interface{"Getter": {Name: "Getter", TypeParams: []string{"T"}}}
	if got := bindingForType(classes, ifaces, memberVarType{name: "Box", args: "Ttt"}); got.args != "Ttt" {
		t.Errorf("class binding = %+v", got)
	}
	if got := bindingForType(classes, ifaces, memberVarType{name: "Getter", args: "Ttt"}); got.args != "Ttt" {
		t.Errorf("iface binding = %+v", got)
	}
	if got := bindingForType(classes, ifaces, memberVarType{name: "Nope"}); got.args != "" {
		t.Errorf("missing binding = %+v", got)
	}
	if got := canonicalTypedMember(classes, ifaces, memberVarType{name: "Box", args: "Ttt", key: "k", kind: containerSequence}); got.args != "Ttt" || got.key != "k" || got.kind != containerSequence {
		t.Errorf("canonicalTypedMember = %+v", got)
	}
	if got := canonicalTypedMember(classes, ifaces, memberVarType{}); !got.empty() {
		t.Errorf("canonicalTypedMember empty = %+v", got)
	}
}
