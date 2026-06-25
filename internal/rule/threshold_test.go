package rule

import (
	"testing"

	"github.com/quality-gates/messgo/internal/model"
)

func TestBoundaryViolates(t *testing.T) {
	cases := []struct {
		name      string
		boundary  Boundary
		value     int
		threshold int
		want      bool
	}{
		// AtOrAbove violates at the threshold and above (value >= threshold).
		{"atOrAbove below", AtOrAbove, 9, 10, false},
		{"atOrAbove at", AtOrAbove, 10, 10, true},
		{"atOrAbove above", AtOrAbove, 11, 10, true},
		// Above violates only strictly above the threshold (value > threshold).
		{"above below", Above, 9, 10, false},
		{"above at", Above, 10, 10, false},
		{"above above", Above, 11, 10, true},
	}
	for _, tc := range cases {
		if got := tc.boundary.Violates(tc.value, tc.threshold); got != tc.want {
			t.Errorf("%s: Violates(%d,%d)=%v, want %v", tc.name, tc.value, tc.threshold, got, tc.want)
		}
	}
}

type thresholdRuleFixture struct {
	*Base
	*ThresholdRule
}

func newThresholdRuleFixture(boundary Boundary) *thresholdRuleFixture {
	r := &thresholdRuleFixture{Base: NewBase()}
	r.RuleName = "FixtureThreshold"
	r.RuleMessage = "{0} {1} has value {2} over {3}"
	r.RulePrio = 3
	r.ThresholdRule = NewThresholdRule(ThresholdDeclaration{
		Property: "limit",
		Default:  10,
		Boundary: boundary,
		NodeKind: ThresholdFunction,
		FuncMetric: func(_ *Context, fn *model.Function) (ThresholdMeasurement, bool) {
			return ThresholdMeasurement{
				Value: len(fn.Params),
				Args:  []any{string(fn.NodeType()), fn.Name},
			}, true
		},
	})
	return r
}

func TestThresholdRuleReportsAtOrAboveBoundary(t *testing.T) {
	r := newThresholdRuleFixture(AtOrAbove)
	if err := r.Configure(Properties{"limit": "2"}); err != nil {
		t.Fatal(err)
	}

	violations := Analyze(thresholdTestFile(2), []*RuleSet{{Rules: []Rule{r}}})

	if len(violations) != 1 {
		t.Fatalf("expected one violation, got %d", len(violations))
	}
	if got, want := violations[0].Args, []any{"function", "sample", 2, 2}; !sameArgs(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestThresholdRuleAllowsStrictBoundaryAtThreshold(t *testing.T) {
	r := newThresholdRuleFixture(Above)
	if err := r.Configure(Properties{"limit": "2"}); err != nil {
		t.Fatal(err)
	}

	violations := Analyze(thresholdTestFile(2), []*RuleSet{{Rules: []Rule{r}}})

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestThresholdRuleRejectsInvalidThresholdAtConfigureTime(t *testing.T) {
	r := newThresholdRuleFixture(AtOrAbove)

	if err := r.Configure(Properties{"limit": "not-an-int"}); err == nil {
		t.Fatal("expected invalid threshold error")
	}
}

func thresholdTestFile(paramCount int) *model.File {
	fn := &model.Function{Name: "sample", Line: 1, EndLine: 1}
	for range paramCount {
		fn.Params = append(fn.Params, &model.Parameter{})
	}
	file := &model.File{Path: "fixture.go", Package: "fixture", AllFuncs: []*model.Function{fn}}
	fn.File = file
	return file
}

func sameArgs(got, want []any) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
