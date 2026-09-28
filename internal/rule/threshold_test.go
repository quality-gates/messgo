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
	*FuncThresholdRule
}

func newThresholdRuleFixture(boundary Boundary) *thresholdRuleFixture {
	r := &thresholdRuleFixture{Base: NewBase()}
	r.RuleName = "FixtureThreshold"
	r.RuleMessage = "{0} {1} has value {2} over {3}"
	r.RulePrio = 3
	r.FuncThresholdRule = NewFuncThresholdRule(ThresholdDeclaration{
		Property: "limit",
		Default:  10,
		Boundary: boundary,
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

func TestAnalyzeDoesNotDispatchFunctionThresholdToStandaloneInterface(t *testing.T) {
	interfaceMeasurements := 0
	functionMeasurements := []string{}
	r := &thresholdRuleFixture{Base: NewBase()}
	r.FuncThresholdRule = NewFuncThresholdRule(ThresholdDeclaration{
		FuncMetric: func(_ *Context, fn *model.Function) (ThresholdMeasurement, bool) {
			functionMeasurements = append(functionMeasurements, fn.Name)
			return ThresholdMeasurement{}, true
		},
		InterfaceMetric: func(_ *Context, _ *model.Interface) (ThresholdMeasurement, bool) {
			interfaceMeasurements++
			return ThresholdMeasurement{}, true
		},
	})

	file, err := model.ParseSource("fixture.go", []byte("package fixture\ntype Reader interface { Read() }\nfunc fetch() {}\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	Analyze(file, []*RuleSet{{Rules: []Rule{r}}})

	if interfaceMeasurements != 0 {
		t.Fatalf("function threshold measured %d standalone interfaces, want 0", interfaceMeasurements)
	}
	if len(functionMeasurements) != 1 || functionMeasurements[0] != "fetch" {
		t.Fatalf("function threshold measured %v, want [fetch]", functionMeasurements)
	}
}

func TestThresholdHelperDoesNotPromoteAwarenessInterfaces(t *testing.T) {
	thresholdRule := Rule(&struct {
		*Base
		*ThresholdRule
	}{Base: NewBase(), ThresholdRule: NewThresholdRule(ThresholdDeclaration{})})
	assertThresholdAwareness(t, thresholdRule, false, false, false)
}

func TestThresholdWrappersDeclareTheirArtifactKinds(t *testing.T) {
	cases := []struct {
		name                   string
		rule                   Rule
		function, class, iface bool
	}{
		{
			name: "function",
			rule: &struct {
				*Base
				*FuncThresholdRule
			}{Base: NewBase(), FuncThresholdRule: NewFuncThresholdRule(ThresholdDeclaration{})},
			function: true,
		},
		{
			name: "class",
			rule: &struct {
				*Base
				*ClassThresholdRule
			}{Base: NewBase(), ClassThresholdRule: NewClassThresholdRule(ThresholdDeclaration{})},
			class: true,
		},
		{
			name: "interface",
			rule: &struct {
				*Base
				*InterfaceThresholdRule
			}{Base: NewBase(), InterfaceThresholdRule: NewInterfaceThresholdRule(ThresholdDeclaration{})},
			iface: true,
		},
		{
			name: "class and interface",
			rule: &struct {
				*Base
				*ClassInterfaceThresholdRule
			}{Base: NewBase(), ClassInterfaceThresholdRule: NewClassInterfaceThresholdRule(ThresholdDeclaration{})},
			class: true,
			iface: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertThresholdAwareness(t, tc.rule, tc.function, tc.class, tc.iface)
		})
	}
}

func TestClassThresholdRuleAppliesClassMetric(t *testing.T) {
	file := thresholdTestArtifacts(t)
	classMeasurements := 0
	r := &struct {
		*Base
		*ClassThresholdRule
	}{Base: newThresholdBase(), ClassThresholdRule: NewClassThresholdRule(ThresholdDeclaration{
		Default:  1,
		Boundary: AtOrAbove,
		ClassMetric: func(_ *Context, class *model.Class) (ThresholdMeasurement, bool) {
			classMeasurements++
			if class.Name != "Widget" {
				t.Errorf("measured class %q, want Widget", class.Name)
			}
			return ThresholdMeasurement{Value: 1}, true
		},
	})}

	violations := Analyze(file, []*RuleSet{{Rules: []Rule{r}}})
	if classMeasurements != 1 || len(violations) != 1 {
		t.Fatalf("class measurements = %d, violations = %d; want 1 each", classMeasurements, len(violations))
	}
}

func TestInterfaceThresholdRuleAppliesInterfaceMetric(t *testing.T) {
	file := thresholdTestArtifacts(t)
	interfaceMeasurements := 0
	r := &struct {
		*Base
		*InterfaceThresholdRule
	}{Base: newThresholdBase(), InterfaceThresholdRule: NewInterfaceThresholdRule(ThresholdDeclaration{
		Boundary:         AtOrAbove,
		InterfaceDefault: 1,
		InterfaceMetric: func(_ *Context, iface *model.Interface) (ThresholdMeasurement, bool) {
			interfaceMeasurements++
			if iface.Name != "Reader" {
				t.Errorf("measured interface %q, want Reader", iface.Name)
			}
			return ThresholdMeasurement{Value: 1}, true
		},
	})}

	violations := Analyze(file, []*RuleSet{{Rules: []Rule{r}}})
	if interfaceMeasurements != 1 || len(violations) != 1 {
		t.Fatalf("interface measurements = %d, violations = %d; want 1 each", interfaceMeasurements, len(violations))
	}
}

func TestClassInterfaceThresholdRuleAppliesBothMetrics(t *testing.T) {
	file := thresholdTestArtifacts(t)
	classMeasurements, interfaceMeasurements := 0, 0
	r := &struct {
		*Base
		*ClassInterfaceThresholdRule
	}{Base: newThresholdBase(), ClassInterfaceThresholdRule: NewClassInterfaceThresholdRule(ThresholdDeclaration{
		Default:          1,
		Boundary:         AtOrAbove,
		InterfaceDefault: 1,
		ClassMetric: func(_ *Context, _ *model.Class) (ThresholdMeasurement, bool) {
			classMeasurements++
			return ThresholdMeasurement{Value: 1}, true
		},
		InterfaceMetric: func(_ *Context, _ *model.Interface) (ThresholdMeasurement, bool) {
			interfaceMeasurements++
			return ThresholdMeasurement{Value: 1}, true
		},
	})}

	violations := Analyze(file, []*RuleSet{{Rules: []Rule{r}}})
	if classMeasurements != 1 || interfaceMeasurements != 1 || len(violations) != 2 {
		t.Fatalf("measurements = class %d/interface %d, violations = %d; want 1/1/2", classMeasurements, interfaceMeasurements, len(violations))
	}
}

func TestThresholdWrappersIgnoreMissingAndUnavailableMetrics(t *testing.T) {
	file := thresholdTestArtifacts(t)
	missingMetrics := []Rule{
		&struct {
			*Base
			*FuncThresholdRule
		}{Base: newThresholdBase(), FuncThresholdRule: NewFuncThresholdRule(ThresholdDeclaration{})},
		&struct {
			*Base
			*ClassThresholdRule
		}{Base: newThresholdBase(), ClassThresholdRule: NewClassThresholdRule(ThresholdDeclaration{})},
		&struct {
			*Base
			*InterfaceThresholdRule
		}{Base: newThresholdBase(), InterfaceThresholdRule: NewInterfaceThresholdRule(ThresholdDeclaration{})},
		&struct {
			*Base
			*ClassInterfaceThresholdRule
		}{Base: newThresholdBase(), ClassInterfaceThresholdRule: NewClassInterfaceThresholdRule(ThresholdDeclaration{})},
	}
	for _, r := range missingMetrics {
		if violations := Analyze(file, []*RuleSet{{Rules: []Rule{r}}}); len(violations) != 0 {
			t.Errorf("%T with missing metrics reported %d violations, want 0", r, len(violations))
		}
	}

	unavailable := &struct {
		*Base
		*ClassInterfaceThresholdRule
	}{Base: newThresholdBase(), ClassInterfaceThresholdRule: NewClassInterfaceThresholdRule(ThresholdDeclaration{
		ClassMetric: func(_ *Context, _ *model.Class) (ThresholdMeasurement, bool) {
			return ThresholdMeasurement{}, false
		},
		InterfaceMetric: func(_ *Context, _ *model.Interface) (ThresholdMeasurement, bool) {
			return ThresholdMeasurement{}, false
		},
	})}
	if violations := Analyze(file, []*RuleSet{{Rules: []Rule{unavailable}}}); len(violations) != 0 {
		t.Fatalf("unavailable metrics reported %d violations, want 0", len(violations))
	}
}

func thresholdTestArtifacts(t *testing.T) *model.File {
	t.Helper()
	file, err := model.ParseSource("fixture.go", []byte(`package fixture

type Widget struct{}
type Reader interface { Read() }
func fetch() {}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return file
}

func newThresholdBase() *Base {
	base := NewBase()
	base.RuleName = "FixtureThreshold"
	base.RuleMessage = "threshold exceeded"
	base.RulePrio = 3
	return base
}

func assertThresholdAwareness(t *testing.T, r Rule, function, class, iface bool) {
	t.Helper()
	if _, ok := r.(FuncRule); ok != function {
		t.Errorf("FuncRule awareness = %t, want %t", ok, function)
	}
	if _, ok := r.(ClassRule); ok != class {
		t.Errorf("ClassRule awareness = %t, want %t", ok, class)
	}
	if _, ok := r.(InterfaceRule); ok != iface {
		t.Errorf("InterfaceRule awareness = %t, want %t", ok, iface)
	}
}

func TestThresholdRulePropertyNames(t *testing.T) {
	var nilRule *ThresholdRule
	if got := nilRule.PropertyNames(); got != nil {
		t.Fatalf("nil receiver PropertyNames = %#v, want nil", got)
	}
	empty := NewThresholdRule(ThresholdDeclaration{})
	if got := empty.PropertyNames(); len(got) != 0 {
		t.Fatalf("empty declaration PropertyNames = %#v, want empty", got)
	}
	only := NewThresholdRule(ThresholdDeclaration{Property: "minimum"})
	if got := only.PropertyNames(); len(got) != 1 || got[0] != "minimum" {
		t.Fatalf("PropertyNames = %#v, want [minimum]", got)
	}
	both := NewThresholdRule(ThresholdDeclaration{Property: "maxmethods", InterfaceProperty: "maxifacemethods"})
	if got := both.PropertyNames(); len(got) != 2 || got[0] != "maxmethods" || got[1] != "maxifacemethods" {
		t.Fatalf("PropertyNames = %#v, want [maxmethods maxifacemethods]", got)
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
