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

func TestOverlappingRulesetsDedupe(t *testing.T) {
	// "go" already imports "codesize", so "go,codesize" must not run any
	// rule twice (which previously emitted every codesize violation twice).
	sets, err := (&Loader{}).Load("go,codesize")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	counts := map[string]int{}
	for _, set := range sets {
		for _, r := range set.Rules {
			counts[r.Name()]++
		}
	}
	for name, n := range counts {
		if n != 1 {
			t.Errorf("rule %s appears %d times across rulesets, want 1", name, n)
		}
	}
	// codesize rules must still be present (dedupe keeps the first copy).
	if counts["CyclomaticComplexity"] != 1 {
		t.Errorf("CyclomaticComplexity present %d times, want 1", counts["CyclomaticComplexity"])
	}
}

func TestOpinionatedRulesNotInDefaultGo(t *testing.T) {
	// These rules conflict with idiomatic Go and live only in the opt-in
	// "opinionated" ruleset, not the default "go" ruleset.
	moved := []string{"ElseExpression", "BooleanArgumentFlag", "UnusedFormalParameter", "GlobalVariable"}

	goSet := loadOne(t, "go")
	for _, name := range moved {
		if ruleByName(goSet, name) != nil {
			t.Errorf("go ruleset should not include %s (it is opinionated, not idiomatic Go)", name)
		}
	}

	opinionated := loadOne(t, "opinionated")
	for _, name := range moved {
		if ruleByName(opinionated, name) == nil {
			t.Errorf("opinionated ruleset should include %s", name)
		}
	}
	if got := len(opinionated.Rules); got != len(moved) {
		t.Errorf("opinionated ruleset has %d rules, want %d", got, len(moved))
	}
}

func ruleNames(sets []*rule.RuleSet) map[string]bool {
	names := map[string]bool{}
	for _, set := range sets {
		for _, r := range set.Rules {
			names[r.Name()] = true
		}
	}
	return names
}

func TestFilterRules(t *testing.T) {
	load := func(t *testing.T) []*rule.RuleSet {
		sets, err := (&Loader{}).Load("codesize")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		return sets
	}

	t.Run("enable keeps only the whitelist", func(t *testing.T) {
		sets := load(t)
		FilterRules(sets, []string{"CyclomaticComplexity", "NPathComplexity"}, nil)
		got := ruleNames(sets)
		if len(got) != 2 || !got["CyclomaticComplexity"] || !got["NPathComplexity"] {
			t.Errorf("enable filter = %v, want only the two named rules", got)
		}
	})

	t.Run("disable removes the blacklist", func(t *testing.T) {
		sets := load(t)
		before := len(ruleNames(sets))
		FilterRules(sets, nil, []string{"CyclomaticComplexity"})
		got := ruleNames(sets)
		if got["CyclomaticComplexity"] {
			t.Error("disabled rule still present")
		}
		if len(got) != before-1 {
			t.Errorf("disable removed %d rules, want 1", before-len(got))
		}
	})

	t.Run("enable then disable", func(t *testing.T) {
		sets := load(t)
		FilterRules(sets, []string{"CyclomaticComplexity", "NPathComplexity"}, []string{"NPathComplexity"})
		got := ruleNames(sets)
		if len(got) != 1 || !got["CyclomaticComplexity"] {
			t.Errorf("enable+disable = %v, want only CyclomaticComplexity", got)
		}
	})

	t.Run("unknown names are ignored", func(t *testing.T) {
		sets := load(t)
		before := len(ruleNames(sets))
		FilterRules(sets, nil, []string{"NoSuchRule"})
		if got := len(ruleNames(sets)); got != before {
			t.Errorf("unknown disable changed rule count: %d -> %d", before, got)
		}
	})

	t.Run("empty filters are a no-op", func(t *testing.T) {
		sets := load(t)
		before := len(ruleNames(sets))
		FilterRules(sets, nil, nil)
		if got := len(ruleNames(sets)); got != before {
			t.Errorf("no-op filter changed rule count: %d -> %d", before, got)
		}
	})
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
