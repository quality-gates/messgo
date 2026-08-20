package unusedcode

import (
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
