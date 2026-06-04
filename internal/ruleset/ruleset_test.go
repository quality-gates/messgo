package ruleset

import (
	"testing"

	"github.com/quality-gates/messgo/internal/rule"
)

func loadOne(t *testing.T, spec string) *rule.RuleSet {
	t.Helper()
	sets, err := (&Loader{}).Load(spec)
	if err != nil {
		t.Fatalf("load %q: %v", spec, err)
	}
	if len(sets) != 1 {
		t.Fatalf("expected 1 ruleset, got %d", len(sets))
	}
	return sets[0]
}

func ruleByName(set *rule.RuleSet, name string) rule.Rule {
	for _, r := range set.Rules {
		if r.Name() == name {
			return r
		}
	}
	return nil
}

func TestBuiltinNamingLoads(t *testing.T) {
	set := loadOne(t, "naming")
	if ruleByName(set, "ShortVariable") == nil {
		t.Error("naming should contain ShortVariable")
	}
}

func TestGoRulesetExcludesAndOverrides(t *testing.T) {
	set := loadOne(t, "go")

	// ShortVariable is excluded entirely.
	if ruleByName(set, "ShortVariable") != nil {
		t.Error("go ruleset should exclude ShortVariable")
	}
	// Design false-positives are excluded.
	if ruleByName(set, "ExitExpression") != nil {
		t.Error("go ruleset should exclude ExitExpression")
	}
	if ruleByName(set, "CountInLoopExpression") != nil {
		t.Error("go ruleset should exclude CountInLoopExpression")
	}
	// Other rules are still present.
	if ruleByName(set, "CyclomaticComplexity") == nil {
		t.Error("go ruleset should still include CyclomaticComplexity")
	}

	// LongVariable is re-added with an overridden maximum (35, not 20).
	lv := ruleByName(set, "LongVariable")
	if lv == nil {
		t.Fatal("go ruleset should include LongVariable (overridden)")
	}
	if got := rule.BaseOf(lv).RuleProps.Int("maximum", 0); got != 35 {
		t.Errorf("LongVariable maximum override = %d, want 35", got)
	}
	// And it must appear exactly once (bulk exclude + single re-add).
	count := 0
	for _, r := range set.Rules {
		if r.Name() == "LongVariable" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("LongVariable appears %d times, want 1", count)
	}
}

func TestMessageTemplatePreserved(t *testing.T) {
	set := loadOne(t, "codesize")
	r := ruleByName(set, "CyclomaticComplexity")
	if r == nil {
		t.Fatal("missing CyclomaticComplexity")
	}
	want := "The {0} {1}() has a Cyclomatic Complexity of {2}. The configured cyclomatic complexity threshold is {3}."
	if r.Message() != want {
		t.Errorf("message = %q, want %q", r.Message(), want)
	}
}
