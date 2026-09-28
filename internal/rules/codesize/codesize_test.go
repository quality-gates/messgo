package codesize

import (
	"fmt"
	"strings"
	"testing"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
)

func analyzeConfiguredRule(t *testing.T, src string, r rule.Rule, props rule.Properties) []*rule.Violation {
	t.Helper()
	configurable, ok := r.(rule.Configurable)
	if !ok {
		t.Fatalf("%T should implement rule.Configurable", r)
	}
	if err := configurable.Configure(props); err != nil {
		t.Fatalf("configure %T: %v", r, err)
	}
	f, err := model.ParseSource("fixture.go", []byte("package fixture\n"+src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{r}}})
}

func TestThresholdRulesAdvertiseOnlyTheirArtifactKinds(t *testing.T) {
	cases := []struct {
		name                   string
		rule                   rule.Rule
		function, class, iface bool
	}{
		{name: "CyclomaticComplexity", rule: newCyclomaticComplexity(), function: true},
		{name: "CognitiveComplexity", rule: newCognitiveComplexity(), function: true},
		{name: "NestingDepth", rule: newNestingDepth(), function: true},
		{name: "ExcessiveReturnCount", rule: newExcessiveReturnCount(), function: true},
		{name: "NPathComplexity", rule: newNPathComplexity(), function: true},
		{name: "LongMethod", rule: newLongMethod(), function: true},
		{name: "LongParameterList", rule: newLongParameterList(), function: true},
		{name: "LongClass", rule: newLongClass(), class: true},
		{name: "ExcessivePublicCount", rule: newExcessivePublicCount(), class: true},
		{name: "TooManyFields", rule: newTooManyFields(), class: true},
		{name: "TooManyMethods", rule: newTooManyMethods(), class: true, iface: true},
		{name: "TooManyPublicMethods", rule: newTooManyPublicMethods(), class: true},
		{name: "WeightedMethodCount", rule: newWeightedMethodCount(), class: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := tc.rule.(rule.FuncRule); ok != tc.function {
				t.Errorf("FuncRule awareness = %t, want %t", ok, tc.function)
			}
			if _, ok := tc.rule.(rule.ClassRule); ok != tc.class {
				t.Errorf("ClassRule awareness = %t, want %t", ok, tc.class)
			}
			if _, ok := tc.rule.(rule.InterfaceRule); ok != tc.iface {
				t.Errorf("InterfaceRule awareness = %t, want %t", ok, tc.iface)
			}
		})
	}
}

func TestThresholdConstructorsWireMeasurements(t *testing.T) {
	src := `type Widget struct {
	Name string
	ID   int
}

func (Widget) RunA() {
	if true {
		println("run")
	}
}

func (Widget) RunB() {}

type Reader interface {
	Read([]byte) (int, error)
	Write([]byte) error
}

func complex(a, b int) (int, error) {
	if a > 0 {
		if b > 0 {
			return 1, nil
		}
	}
	return 0, nil
}
`
	cases := []struct {
		name  string
		new   func() rule.Rule
		props rule.Properties
	}{
		{name: "CyclomaticComplexity", new: newCyclomaticComplexity, props: rule.Properties{"reportLevel": "0"}},
		{name: "CognitiveComplexity", new: newCognitiveComplexity, props: rule.Properties{"reportLevel": "0"}},
		{name: "NestingDepth", new: newNestingDepth, props: rule.Properties{"maxdepth": "0"}},
		{name: "ExcessiveReturnCount", new: newExcessiveReturnCount, props: rule.Properties{"maxresults": "0"}},
		{name: "NPathComplexity", new: newNPathComplexity, props: rule.Properties{"minimum": "0"}},
		{name: "LongMethod", new: newLongMethod, props: rule.Properties{"minimum": "0"}},
		{name: "LongParameterList", new: newLongParameterList, props: rule.Properties{"minimum": "0"}},
		{name: "LongClass", new: newLongClass, props: rule.Properties{"minimum": "0"}},
		{name: "ExcessivePublicCount", new: newExcessivePublicCount, props: rule.Properties{"minimum": "0"}},
		{name: "TooManyFields", new: newTooManyFields, props: rule.Properties{"maxfields": "0"}},
		{name: "TooManyMethods", new: newTooManyMethods, props: rule.Properties{"maxmethods": "0", "maxifacemethods": "0"}},
		{name: "TooManyPublicMethods", new: newTooManyPublicMethods, props: rule.Properties{"maxmethods": "0"}},
		{name: "WeightedMethodCount", new: newWeightedMethodCount, props: rule.Properties{"maximum": "0"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if violations := analyzeConfiguredRule(t, src, tc.new(), tc.props); len(violations) == 0 {
				t.Fatal("configured threshold reported no violations")
			}
		})
	}
}

func TestCyclomaticComplexityFlagsByDefault(t *testing.T) {
	violations := analyzeConfiguredRule(t, `
func complexFunc(x int) {
	if x > 0 {
		println(x)
	}
}
`, newCyclomaticComplexity(), rule.Properties{"reportLevel": "2"})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
}

func TestCyclomaticComplexityShowMethodsComplexityFalseSuppresses(t *testing.T) {
	violations := analyzeConfiguredRule(t, `
func complexFunc(x int) {
	if x > 0 {
		println(x)
	}
}
`, newCyclomaticComplexity(), rule.Properties{"reportLevel": "2", "showMethodsComplexity": "false"})
	if len(violations) != 0 {
		t.Fatalf("expected showMethodsComplexity=false to suppress the violation, got %d", len(violations))
	}
}

func BenchmarkWhitespaceAwareLOC(b *testing.B) {
	for _, classes := range []int{100, 200, 400} {
		b.Run(fmt.Sprintf("classes_%d", classes), func(b *testing.B) {
			methodRule := newLongMethod().(*LongMethod)
			classRule := newLongClass().(*LongClass)
			props := rule.Properties{
				"minimum":           "1000000",
				"ignore-whitespace": "true",
			}
			if err := methodRule.Configure(props); err != nil {
				b.Fatal(err)
			}
			if err := classRule.Configure(props); err != nil {
				b.Fatal(err)
			}
			sets := []*rule.RuleSet{{Rules: []rule.Rule{methodRule, classRule}}}
			b.ResetTimer()
			for b.Loop() {
				file := whitespaceAwareLOCFile(b, classes)
				if violations := rule.Analyze(file, sets); len(violations) != 0 {
					b.Fatalf("violations = %d, want 0", len(violations))
				}
			}
		})
	}
}

func whitespaceAwareLOCFile(tb testing.TB, classes int) *model.File {
	tb.Helper()
	var src strings.Builder
	src.WriteString("package sample\n")
	for index := range classes {
		fmt.Fprintf(&src, "type type%d struct {\n\t// field\n\n\tvalue int\n}\n", index)
		fmt.Fprintf(&src, "func (type%d) method%d() {\n\t// body\n\n\t_ = %d\n}\n", index, index, index)
	}
	file, err := model.ParseSource("codesize.go", []byte(src.String()))
	if err != nil {
		tb.Fatalf("ParseSource: %v", err)
	}
	return file
}
