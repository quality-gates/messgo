package cleancode

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

func TestBooleanArgumentFlagFlagsByDefault(t *testing.T) {
	violations := analyzeConfiguredRule(t, `
type S struct{}

func (s *S) SetEnabled(flag bool) {}
`, newBooleanArgumentFlag(), rule.Properties{})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
}

func TestBooleanArgumentFlagExceptions(t *testing.T) {
	violations := analyzeConfiguredRule(t, `
type S struct{}

func (s *S) SetEnabled(flag bool) {}
`, newBooleanArgumentFlag(), rule.Properties{"exceptions": "S"})
	if len(violations) != 0 {
		t.Fatalf("expected exceptions to suppress the receiver, got %d violations", len(violations))
	}
}

func TestBooleanArgumentFlagIgnorePattern(t *testing.T) {
	violations := analyzeConfiguredRule(t, `
type S struct{}

func (s *S) SetEnabled(flag bool) {}
`, newBooleanArgumentFlag(), rule.Properties{"ignorepattern": "^Set.*"})
	if len(violations) != 0 {
		t.Fatalf("expected ignorepattern to suppress SetEnabled, got %d violations", len(violations))
	}
}

func TestBooleanArgumentFlagIgnorePatternNoMatch(t *testing.T) {
	violations := analyzeConfiguredRule(t, `
type S struct{}

func (s *S) SetEnabled(flag bool) {}
`, newBooleanArgumentFlag(), rule.Properties{"ignorepattern": "^Get.*"})
	if len(violations) != 1 {
		t.Fatalf("expected ignorepattern mismatch to still flag SetEnabled, got %d violations", len(violations))
	}
}
