package opinionated

import (
	"slices"
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

func violationLines(vs []*rule.Violation) []int {
	var lines []int
	for _, v := range vs {
		lines = append(lines, v.BeginLine)
	}
	return lines
}

func assertIdenticalBranchesLines(t *testing.T, src string, want ...int) {
	t.Helper()
	got := violationLines(analyzeSource(t, newIdenticalBranches(), src))
	if !slices.Equal(got, want) {
		t.Fatalf("IdenticalBranches lines = %v, want %v", got, want)
	}
}

func TestIdenticalBranchesElseIfChainWithElse(t *testing.T) {
	assertIdenticalBranchesLines(t, `package sample

func f(a, b, c bool) {
	if a {
		println("same")
	} else if b {
		println("other")
	} else if c {
		println("same")
	} else {
		println("other")
	}
}
`, 8, 10)
}

func TestIdenticalBranchesElseIfChainReportsEachDuplicateOnce(t *testing.T) {
	assertIdenticalBranchesLines(t, `package sample

func f(a, b bool) {
	if a {
		println("same")
	} else if b {
		println("same")
	} else {
		println("same")
	}
}
`, 6, 8)
}

func TestIdenticalBranchesElseIfChainSkipsEmptyBranches(t *testing.T) {
	assertIdenticalBranchesLines(t, `package sample

func f(a, b, c bool) {
	if a {
	} else if b {
	} else if c {
		println("same")
	} else if a == b {
		println("same")
	}
}
`, 8)
}

func TestIdenticalBranchesNestedIfInsideBranch(t *testing.T) {
	assertIdenticalBranchesLines(t, `package sample

func f(a, b bool) {
	if a {
		if b {
			println("inner")
		} else {
			println("inner")
		}
	} else {
		println("outer")
	}
}
`, 7)
}

func TestIdenticalBranchesSwitchReportsEachDuplicateOnce(t *testing.T) {
	assertIdenticalBranchesLines(t, `package sample

func f(x int) {
	switch x {
	case 1:
		println("same")
	case 2:
		println("same")
	case 3:
		println("same")
	}
}
`, 7, 9)
}

func TestIdenticalBranchesTypeSwitch(t *testing.T) {
	assertIdenticalBranchesLines(t, `package sample

func f(v any) {
	switch v.(type) {
	case int:
		println("same")
	case string:
		println("other")
	default:
		println("same")
	}
}
`, 9)
}

func TestUncheckedTypeAssertionParenthesizedCommaOK(t *testing.T) {
	r := newUncheckedTypeAssertion()
	src := `package sample

func f(x any) {
	v, ok := (x.(int))
	_ = v
	_ = ok
}

func fNested(x any) {
	v, ok := ((x.(int)))
	_ = v
	_ = ok
}

func fVar(x any) {
	var v, ok = (x.(int))
	_ = v
	_ = ok
}

func fIf(x any) {
	if v, ok := (x.(int)); ok {
		_ = v
	}
}
`
	violations := analyzeSource(t, r, src)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d", len(violations))
	}
}

func TestUncheckedTypeAssertionParenthesizedUnchecked(t *testing.T) {
	r := newUncheckedTypeAssertion()
	src := `package sample

func f(x any) {
	v := (x.(int))
	_ = v
}

func fNested(x any) {
	v := ((x.(int)))
	_ = v
}
`
	violations := analyzeSource(t, r, src)
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(violations))
	}
}
