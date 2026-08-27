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
		_ = duplicate + ignored
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

func TestUnusedVariableRulesHandleBindingAndDeclarationForms(t *testing.T) {
	f, err := model.ParseSource("unused.go", []byte(`package sample

func inspect(unusedParam, capturedParam int, values []int) {
	read, writeOnly := 1, 2
	_ = read
	writeOnly = 3
	for usedKey, unusedValue := range values {
		_ = usedKey
	}
	captured := 1
	closure := func() {
		unusedParam := 1
		_ = unusedParam
		_ = capturedParam
		_ = captured
	}
	_ = closure
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	formalRule := &UnusedFormalParameter{Base: rule.NewBase()}
	localRule := newUnusedLocalVariable().(*UnusedLocalVariable)
	violations := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{formalRule, localRule}}})
	var formalNames, localNames []string
	for _, violation := range violations {
		name := violation.Args[0].(string)
		switch violation.Rule.(type) {
		case *UnusedFormalParameter:
			formalNames = append(formalNames, name)
		case *UnusedLocalVariable:
			localNames = append(localNames, name)
		}
	}
	if fmt.Sprint(formalNames) != "[unusedParam]" {
		t.Fatalf("formal parameter violations = %v, want [unusedParam]", formalNames)
	}
	if fmt.Sprint(localNames) != "[writeOnly unusedValue]" {
		t.Fatalf("local variable violations = %v, want [writeOnly unusedValue]", localNames)
	}
}

func TestUnusedMemberRulesRecognizeFileWideMemberUses(t *testing.T) {
	f, err := model.ParseSource("unused.go", []byte(`package sample

const mapKey = "key"

type widget struct {
	selected int
	literal  int
	mapKey  int
	unused  int
}

func (w *widget) selectedMethod() {}
func (w *widget) unusedMethod()   {}

func inspect(w widget) {
	_ = w.selected
	w.selectedMethod()
	_ = widget{literal: 1}
	_ = map[string]int{mapKey: 1}
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	fieldRule := &UnusedPrivateField{Base: rule.NewBase()}
	methodRule := &UnusedPrivateMethod{Base: rule.NewBase()}
	violations := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{fieldRule, methodRule}}})
	got := map[string]bool{}
	for _, violation := range violations {
		var ruleName string
		switch violation.Rule.(type) {
		case *UnusedPrivateField:
			ruleName = "UnusedPrivateField"
		case *UnusedPrivateMethod:
			ruleName = "UnusedPrivateMethod"
		default:
			t.Fatalf("unexpected rule type %T", violation.Rule)
		}
		got[fmt.Sprintf("%s:%v", ruleName, violation.Args[0])] = true
	}

	want := []string{
		"UnusedPrivateField:mapKey",
		"UnusedPrivateField:unused",
		"UnusedPrivateMethod:unusedMethod",
	}
	if len(got) != len(want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
	for _, violation := range want {
		if !got[violation] {
			t.Errorf("missing violation %s; got %v", violation, got)
		}
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
