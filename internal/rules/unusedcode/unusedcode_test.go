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
