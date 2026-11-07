package cli

import (
	"strings"
	"testing"
)

func TestConflictingOverridesWithInheritedRule(t *testing.T) {
	const inherited = `<rule ref="naming"/>`
	const defaultOverride = `<rule ref="naming/LongVariable"><priority>3</priority></rule>`
	const otherOverride = `<rule ref="naming/LongVariable"><priority>2</priority></rule>`

	tests := []struct {
		name    string
		rules   []string
		invalid bool
	}{
		{"inherited-default-other", []string{inherited, defaultOverride, otherOverride}, true},
		{"inherited-other-default", []string{inherited, otherOverride, defaultOverride}, true},
		{"default-inherited-other", []string{defaultOverride, inherited, otherOverride}, true},
		{"other-inherited-default", []string{otherOverride, inherited, defaultOverride}, true},
		{"default-other-inherited", []string{defaultOverride, otherOverride, inherited}, true},
		{"other-default-inherited", []string{otherOverride, defaultOverride, inherited}, true},
		{"identical-overrides", []string{inherited, defaultOverride, defaultOverride}, false},
		{"one-default-override", []string{inherited, defaultOverride}, false},
		{"one-other-override", []string{inherited, otherOverride}, false},
	}
	src := writeFixture(t, "package p\nfunc f() {}\n")
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			xml := `<ruleset name="team">` + strings.Join(test.rules, "\n") + `</ruleset>`
			policy := writeRulesetFile(t, xml)
			code, _, errOut := runMain(t, src, "text", policy)
			if test.invalid {
				if code != ExitError || !strings.Contains(errOut, `conflicting overrides for rule "LongVariable"`) {
					t.Fatalf("exit = %d, stderr = %q; want conflicting override error", code, errOut)
				}
			} else if code != ExitSuccess {
				t.Fatalf("exit = %d, stderr = %q; want success", code, errOut)
			}
		})
	}
}
