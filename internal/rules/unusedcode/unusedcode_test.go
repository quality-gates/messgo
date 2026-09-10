package unusedcode

import (
	"fmt"
	"strings"
	"testing"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
)

func TestUnusedVariableRulesUseBindingAwareReads(t *testing.T) {
	f, err := model.ParseSource("unused.go", []byte(`package sample

func inspect(unused, used int, _ int) {
	read := used
	_ = read
	ignored := 1
	if true {
		duplicate := 1
		_ = duplicate
	}
	duplicate := 2
	closure := func() {
		unused := 1
		_ = unused
		_ = used
	}
	_ = closure
	_ = duplicate
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	formal := &UnusedFormalParameter{Base: rule.NewBase()}
	local := newUnusedLocalVariable().(*UnusedLocalVariable)
	if err := local.Configure(rule.Properties{"exceptions": "ignored"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	sets := []*rule.RuleSet{{Rules: []rule.Rule{formal, local}}}
	violations := rule.Analyze(f, sets)

	var formalNames, localNames []string
	for _, violation := range violations {
		if len(violation.Args) != 1 {
			t.Fatalf("violation args = %+v, want one variable name", violation.Args)
		}
		name, ok := violation.Args[0].(string)
		if !ok {
			t.Fatalf("violation argument type = %T, want string", violation.Args[0])
		}
		switch violation.Rule.(type) {
		case *UnusedFormalParameter:
			formalNames = append(formalNames, name)
		case *UnusedLocalVariable:
			localNames = append(localNames, name)
		default:
			t.Fatalf("unexpected rule type %T", violation.Rule)
		}
	}

	if len(formalNames) != 1 || formalNames[0] != "unused" {
		t.Fatalf("formal parameter violations = %v, want [unused]", formalNames)
	}
	if len(localNames) != 0 {
		t.Fatalf("local variable violations = %v, want none because duplicate is read and ignored is excepted", localNames)
	}
}

func TestUnusedLocalVariableReportsWriteOnlyDuplicateOnce(t *testing.T) {
	f, err := model.ParseSource("unused.go", []byte(`package sample

func inspect() {
	if true {
		duplicate := 1
	}
	duplicate := 2
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	local := newUnusedLocalVariable().(*UnusedLocalVariable)
	violations := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{local}}})
	if len(violations) != 1 || violations[0].Args[0] != "duplicate" {
		t.Fatalf("violations = %+v, want one duplicate violation", violations)
	}
}

func TestUnusedPrivateMembersAreScopedToTheirType(t *testing.T) {
	f, err := model.ParseSource("issue93.go", []byte(`package p

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

	fieldRule := &UnusedPrivateField{Base: rule.NewBase()}
	methodRule := &UnusedPrivateMethod{Base: rule.NewBase()}
	violations := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{fieldRule, methodRule}}})
	if len(violations) != 2 {
		t.Fatalf("violations = %+v, want exactly S.secret and S.do", violations)
	}

	want := map[string]bool{
		"field:4:secret": true,
		"method:S:do":    true,
	}
	for _, violation := range violations {
		name, ok := violation.Args[0].(string)
		if !ok {
			t.Fatalf("violation args = %+v, want a member name", violation.Args)
		}
		key := ""
		if violation.Method == "" {
			key = fmt.Sprintf("field:%d:%s", violation.BeginLine, name)
		} else {
			key = fmt.Sprintf("method:%s:%s", violation.Class, violation.Method)
		}
		if !want[key] {
			t.Errorf("unexpected violation = %+v", violation)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Errorf("missing violations = %v", want)
	}
}

func TestUnusedPrivateMethodSatisfiedBySamePackageInterface(t *testing.T) {
	f, err := model.ParseSource("a.go", []byte(`package p

type Node interface {
	Pos() int
	node()
}

type Leaf struct{ p int }

func (l *Leaf) Pos() int { return l.p }
func (l *Leaf) node()    {}

func Walk(n Node) int { return n.Pos() }
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	methodRule := &UnusedPrivateMethod{Base: rule.NewBase()}
	violations := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{methodRule}}})
	if len(violations) != 0 {
		t.Fatalf("violations = %+v, want none: node() is required for *Leaf to satisfy Node", violations)
	}
}

func TestUnusedPrivateMethodSatisfiedThroughEmbeddedInterface(t *testing.T) {
	f, err := model.ParseSource("a.go", []byte(`package p

type base interface{ hidden() }

type Node interface {
	base
	Pos() int
}

type Leaf struct{}

func (l *Leaf) Pos() int  { return 0 }
func (l *Leaf) hidden()   {}

var _ Node = (*Leaf)(nil)
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	methodRule := &UnusedPrivateMethod{Base: rule.NewBase()}
	violations := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{methodRule}}})
	if len(violations) != 0 {
		t.Fatalf("violations = %+v, want none: hidden() is inherited by Node through base", violations)
	}
}

func TestUnusedPrivateMethodNotSuppressedByIncompatibleInterfaceSignature(t *testing.T) {
	cases := []struct {
		name   string
		iface  string
		method string
	}{
		{
			name:   "parameter count",
			iface:  "do(int)",
			method: "func (s S) do() {}",
		},
		{
			name:   "parameter type",
			iface:  "do(int)",
			method: "func (s S) do(string) {}",
		},
		{
			name:   "result count",
			iface:  "do() int",
			method: "func (s S) do() {}",
		},
		{
			name:   "result type",
			iface:  "do() int",
			method: "func (s S) do() string { return \"\" }",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := model.ParseSource("a.go", []byte(`package p

type Wants interface { `+tc.iface+` }

type S struct{}

`+tc.method+`
`))
			if err != nil {
				t.Fatalf("ParseSource: %v", err)
			}

			methodRule := &UnusedPrivateMethod{Base: rule.NewBase()}
			violations := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{methodRule}}})
			if len(violations) != 1 || violations[0].Args[0] != "do" {
				t.Fatalf("violations = %+v, want one 'do' violation: S cannot implement Wants with an incompatible signature", violations)
			}
		})
	}
}

func TestUnusedPrivateMethodSuppressedByMatchingInterfaceSignature(t *testing.T) {
	f, err := model.ParseSource("a.go", []byte(`package p

type Wants interface {
	do(int) error
	other()
}

type S struct{}

func (s S) do(i int) error { return nil }
func (s S) other()         {}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	methodRule := &UnusedPrivateMethod{Base: rule.NewBase()}
	violations := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{methodRule}}})
	if len(violations) != 0 {
		t.Fatalf("violations = %+v, want none: do(int) error and other() match Wants", violations)
	}
}

func TestUnusedPrivateMethodSelectionAndInterfaceInteractions(t *testing.T) {
	f, err := model.ParseSource("a.go", []byte(`package p

type Node interface{ node() }

type Leaf struct{}

func (l *Leaf) node()    {}
func (l *Leaf) chosen()  {}
func (l *Leaf) orphan()  {}

func walk(l *Leaf) { l.chosen() }
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	methodRule := &UnusedPrivateMethod{Base: rule.NewBase()}
	violations := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{methodRule}}})
	var names []string
	for _, v := range violations {
		if len(v.Args) == 1 {
			if name, ok := v.Args[0].(string); ok {
				names = append(names, name)
			}
		}
	}
	if len(names) != 1 || names[0] != "orphan" {
		t.Fatalf("violations = %v, want only [orphan]: node() satisfies Node and chosen() is selected", names)
	}
}

func BenchmarkUnusedMemberRules(b *testing.B) {
	for _, classes := range []int{100, 200, 400} {
		b.Run(fmt.Sprintf("classes_%d", classes), func(b *testing.B) {
			fieldRule := &UnusedPrivateField{Base: rule.NewBase()}
			methodRule := &UnusedPrivateMethod{Base: rule.NewBase()}
			sets := []*rule.RuleSet{{Rules: []rule.Rule{fieldRule, methodRule}}}
			b.ResetTimer()
			for b.Loop() {
				file := unusedMemberFile(b, classes)
				if got := len(rule.Analyze(file, sets)); got != classes*2 {
					b.Fatalf("violations = %d, want %d", got, classes*2)
				}
			}
		})
	}
}

func BenchmarkUnusedVariableRules(b *testing.B) {
	for _, variables := range []int{50, 100, 200} {
		b.Run(fmt.Sprintf("variables_%d", variables), func(b *testing.B) {
			formalRule := &UnusedFormalParameter{Base: rule.NewBase()}
			localRule := newUnusedLocalVariable().(*UnusedLocalVariable)
			sets := []*rule.RuleSet{{Rules: []rule.Rule{formalRule, localRule}}}
			b.ResetTimer()
			for b.Loop() {
				file := unusedVariableFile(b, variables)
				if got := len(rule.Analyze(file, sets)); got != variables*2 {
					b.Fatalf("violations = %d, want %d", got, variables*2)
				}
			}
		})
	}
}

func unusedMemberFile(tb testing.TB, classes int) *model.File {
	tb.Helper()
	var src strings.Builder
	src.WriteString("package sample\n")
	for index := range classes {
		fmt.Fprintf(&src, "type type%d struct { field%d int }\n", index, index)
		fmt.Fprintf(&src, "func (*type%d) method%d() {}\n", index, index)
	}
	file, err := model.ParseSource("unused.go", []byte(src.String()))
	if err != nil {
		tb.Fatalf("ParseSource: %v", err)
	}
	return file
}

func unusedVariableFile(tb testing.TB, variables int) *model.File {
	tb.Helper()
	var src strings.Builder
	src.WriteString("package sample\nfunc inspect(")
	for index := range variables {
		if index > 0 {
			src.WriteString(", ")
		}
		fmt.Fprintf(&src, "param%d int", index)
	}
	src.WriteString(") {\n")
	for index := range variables {
		fmt.Fprintf(&src, "var local%d int\n", index)
	}
	src.WriteString("}\n")
	file, err := model.ParseSource("unused.go", []byte(src.String()))
	if err != nil {
		tb.Fatalf("ParseSource: %v", err)
	}
	return file
}
