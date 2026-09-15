package controversial

import (
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

func TestCamelCaseWithOptions(t *testing.T) {
	tests := []struct {
		name                string
		identifier          string
		allowUnderscore     bool
		allowUnderscoreTest bool
		wantValid           bool
	}{
		{name: "camel case", identifier: "privateMethod", wantValid: true},
		{name: "leading underscore allowed", identifier: "_privateMethod", allowUnderscore: true, wantValid: true},
		{name: "leading underscore disabled", identifier: "_privateMethod", wantValid: false},
		{name: "internal underscore", identifier: "private_method", allowUnderscore: true, wantValid: false},
		{name: "multiple leading underscores", identifier: "__privateMethod", allowUnderscore: true, wantValid: false},
		{name: "underscore only", identifier: "_", allowUnderscore: true, wantValid: false},
		{name: "test underscore allowed", identifier: "Test_Feature", allowUnderscoreTest: true, wantValid: true},
		{name: "test underscore disabled", identifier: "Test_Feature", wantValid: false},
		{name: "test option does not relax ordinary names", identifier: "private_method", allowUnderscoreTest: true, wantValid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCamelCaseWithOptions(tt.identifier, tt.allowUnderscore, tt.allowUnderscoreTest); got != tt.wantValid {
				t.Fatalf("isCamelCaseWithOptions(%q, %t, %t) = %t, want %t", tt.identifier, tt.allowUnderscore, tt.allowUnderscoreTest, got, tt.wantValid)
			}
		})
	}
}

func TestCamelCaseRulesConfigureAllowUnderscore(t *testing.T) {
	tests := []struct {
		name    string
		newRule func() rule.Rule
		src     string
	}{
		{
			name:    "method",
			newRule: func() rule.Rule { return &CamelCaseMethodName{Base: rule.NewBase()} },
			src:     "type T struct{}\n\nfunc (t *T) _privateMethod() {}",
		},
		{
			name:    "property",
			newRule: func() rule.Rule { return &CamelCasePropertyName{Base: rule.NewBase()} },
			src:     "type T struct { _ int; _privateField int }",
		},
		{
			name:    "parameter",
			newRule: func() rule.Rule { return &CamelCaseParameterName{Base: rule.NewBase()} },
			src:     "func process(_, _parameterName int) {}",
		},
		{
			name:    "variable",
			newRule: func() rule.Rule { return &CamelCaseVariableName{Base: rule.NewBase()} },
			src:     "func process() { _, _localName := 1, 2; _ = _localName }",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(analyzeConfiguredRule(t, tt.src, tt.newRule(), rule.Properties{"allow-underscore": "true"})); got != 0 {
				t.Fatalf("allow-underscore produced %d violations, want 0", got)
			}
			if got := len(analyzeConfiguredRule(t, tt.src, tt.newRule(), rule.Properties{})); got != 1 {
				t.Fatalf("default underscore policy produced %d violations, want 1", got)
			}
		})
	}
}

func TestCamelCaseRulesConfigureAllowUnderscoreTest(t *testing.T) {
	tests := []struct {
		name    string
		newRule func() rule.Rule
		src     string
	}{
		{
			name:    "method",
			newRule: func() rule.Rule { return &CamelCaseMethodName{Base: rule.NewBase()} },
			src:     "type T struct{}\n\nfunc (t *T) Test_Feature() {}",
		},
		{
			name:    "property",
			newRule: func() rule.Rule { return &CamelCasePropertyName{Base: rule.NewBase()} },
			src:     "type T struct { Test_Feature int }",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(analyzeConfiguredRule(t, tt.src, tt.newRule(), rule.Properties{"allow-underscore-test": "true"})); got != 0 {
				t.Fatalf("allow-underscore-test produced %d violations, want 0", got)
			}
			if got := len(analyzeConfiguredRule(t, tt.src, tt.newRule(), rule.Properties{})); got != 1 {
				t.Fatalf("default test underscore policy produced %d violations, want 1", got)
			}
		})
	}

	if got := len(analyzeConfiguredRule(t, "func private_method() {}", &CamelCaseMethodName{Base: rule.NewBase()}, rule.Properties{"allow-underscore-test": "true"})); got != 1 {
		t.Fatalf("allow-underscore-test produced %d violations for an ordinary method, want 1", got)
	}
}
