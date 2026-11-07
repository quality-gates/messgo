package ruleset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInheritedRuleAndExplicitOverrideOrder(t *testing.T) {
	const inherited = `<rule ref="naming"/>`
	const inheritedRule = `<rule ref="naming/LongVariable"/>`
	const defaultOverride = `<rule ref="naming/LongVariable"><priority>3</priority></rule>`
	const otherOverride = `<rule ref="naming/LongVariable"><priority>2</priority></rule>`

	tests := []struct {
		name         string
		rules        []string
		wantPriority int
		wantConflict bool
	}{
		{"inherited-default-other", []string{inherited, defaultOverride, otherOverride}, 0, true},
		{"inherited-other-default", []string{inherited, otherOverride, defaultOverride}, 0, true},
		{"default-inherited-other", []string{defaultOverride, inherited, otherOverride}, 0, true},
		{"other-inherited-default", []string{otherOverride, inherited, defaultOverride}, 0, true},
		{"default-other-inherited", []string{defaultOverride, otherOverride, inherited}, 0, true},
		{"other-default-inherited", []string{otherOverride, defaultOverride, inherited}, 0, true},
		{"equal-default-overrides", []string{inherited, defaultOverride, defaultOverride}, 3, false},
		{"equal-other-overrides", []string{inherited, otherOverride, otherOverride}, 2, false},
		{"one-default-override", []string{inherited, defaultOverride}, 3, false},
		{"one-other-override", []string{inherited, otherOverride}, 2, false},
		{"duplicate-inherited-then-override", []string{inherited, inherited, otherOverride}, 2, false},
		{"inherited-rule-then-override", []string{inherited, inheritedRule, otherOverride}, 2, false},
		{"equal-overrides-then-conflict", []string{inherited, defaultOverride, defaultOverride, otherOverride}, 0, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "team.xml")
			xml := `<ruleset name="team">` + strings.Join(test.rules, "\n") + `</ruleset>`
			if err := os.WriteFile(path, []byte(xml), 0o600); err != nil {
				t.Fatal(err)
			}

			sets, err := (&Loader{}).Load(path)
			if test.wantConflict {
				if err == nil || !strings.Contains(err.Error(), `conflicting overrides for rule "LongVariable"`) {
					t.Fatalf("load error = %v; want conflicting LongVariable overrides", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if len(sets) != 1 {
				t.Fatalf("got %d rulesets; want 1", len(sets))
			}
			got := ruleByName(sets[0], "LongVariable")
			if got == nil {
				t.Fatal("LongVariable was not loaded")
			}
			if got.Priority() != test.wantPriority {
				t.Errorf("LongVariable priority = %d; want %d", got.Priority(), test.wantPriority)
			}
		})
	}
}

func TestEqualOverridesAcrossRulesets(t *testing.T) {
	tests := []struct {
		name     string
		priority string
		want     int
	}{
		{"default-priority", "3", 3},
		{"other-priority", "2", 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			var paths []string
			for _, name := range []string{"first.xml", "second.xml"} {
				path := filepath.Join(dir, name)
				xml := `<ruleset name="team"><rule ref="naming/LongVariable"><priority>` + test.priority + `</priority></rule></ruleset>`
				if err := os.WriteFile(path, []byte(xml), 0o600); err != nil {
					t.Fatal(err)
				}
				paths = append(paths, path)
			}

			sets, err := (&Loader{}).Load(strings.Join(paths, ","))
			if err != nil {
				t.Fatalf("load equal overrides: %v", err)
			}
			count := 0
			for _, set := range sets {
				for _, r := range set.Rules {
					if r.Name() == "LongVariable" {
						count++
						if r.Priority() != test.want {
							t.Errorf("LongVariable priority = %d; want %d", r.Priority(), test.want)
						}
					}
				}
			}
			if count != 1 {
				t.Errorf("loaded %d LongVariable rules; want 1", count)
			}
		})
	}
}
