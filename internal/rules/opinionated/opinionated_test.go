package opinionated

import (
	"testing"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
)

func analyzeSource(t *testing.T, r rule.Rule, src string) []*rule.Violation {
	t.Helper()
	f, err := model.ParseSource("sample.go", []byte(src))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	sets := []*rule.RuleSet{{Rules: []rule.Rule{r}}}
	return rule.Analyze(f, sets)
}

func TestIdenticalBranchesEmptyIfElse(t *testing.T) {
	r := newIdenticalBranches()
	src := `package sample

func f(x int) {
	if x > 0 {
	} else {
	}
}

func fEmptyIf(x int) {
	if x > 0 {
	} else {
		println("else")
	}
}

func fEmptyElse(x int) {
	if x > 0 {
		println("if")
	} else {
	}
}
`
	violations := analyzeSource(t, r, src)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations for empty branches, got %d", len(violations))
	}
}

func TestIdenticalBranchesNonEmptyIfElse(t *testing.T) {
	r := newIdenticalBranches()
	src := `package sample

func f(x int) {
	if x > 0 {
		println("same")
	} else {
		println("same")
	}
}
`
	violations := analyzeSource(t, r, src)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation for identical if/else, got %d", len(violations))
	}
}

func TestIdenticalBranchesDifferentIfElse(t *testing.T) {
	r := newIdenticalBranches()
	src := `package sample

func f(x int) {
	if x > 0 {
		println("one")
	} else {
		println("two")
	}
}
`
	violations := analyzeSource(t, r, src)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations for different if/else, got %d", len(violations))
	}
}

func TestIdenticalBranchesSwitch(t *testing.T) {
	r := newIdenticalBranches()
	src := `package sample

func f(x int) {
	switch x {
	case 1:
	case 2:
	}
}
`
	violations := analyzeSource(t, r, src)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations for empty switch cases, got %d", len(violations))
	}
}
