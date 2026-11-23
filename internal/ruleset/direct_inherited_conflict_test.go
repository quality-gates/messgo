package ruleset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInheritedRuleDoesNotHideConflictingDirectDefinitions(t *testing.T) {
	const inherited = `<rule ref="base.xml"/>`
	const direct3 = `<rule name="LongVariable" class="PHPMD\Rule\Naming\LongVariable"><priority>3</priority></rule>`
	const direct2 = `<rule name="LongVariable" class="PHPMD\Rule\Naming\LongVariable"><priority>2</priority></rule>`
	const override = `<rule ref="naming/LongVariable"><priority>2</priority></rule>`
	tests := []struct {
		name     string
		rules    []string
		conflict bool
	}{
		{"inherited-equal-different", []string{inherited, direct3, direct2}, true},
		{"inherited-different-equal", []string{inherited, direct2, direct3}, true},
		{"direct-inherited-direct", []string{direct3, inherited, direct2}, true},
		{"direct-direct-inherited", []string{direct3, direct2, inherited}, true},
		{"override-direct-direct", []string{override, direct3, direct2}, true},
		{"direct-override-direct", []string{direct3, override, direct2}, true},
		{"equal-directs", []string{inherited, direct3, direct3}, false},
		{"single-different-direct", []string{inherited, direct2}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			base := `<ruleset name="base">` + direct3 + `</ruleset>`
			policy := `<ruleset name="repro">` + strings.Join(test.rules, "") + `</ruleset>`
			for name, content := range map[string]string{"base.xml": base, "policy.xml": policy} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := (&Loader{}).Load(filepath.Join(dir, "policy.xml"))
			if test.conflict {
				if err == nil || !strings.Contains(err.Error(), `conflicting definitions for rule "LongVariable"`) {
					t.Fatalf("load error = %v; want conflicting LongVariable definitions", err)
				}
			} else if err != nil {
				t.Fatalf("load: %v", err)
			}
		})
	}
}
