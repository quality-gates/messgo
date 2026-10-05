package ruleset

import (
	"fmt"
	"slices"

	"github.com/quality-gates/messgo/internal/rule"
)

// ruleSelection applies a Loader's name filters and priority bounds during
// one Load and reports name filters that select nothing.
type ruleSelection struct {
	loader *Loader
	// loaded records every rule name that survived the priority bounds,
	// including rules skipped by the name filters.
	loaded map[string]bool
}

// selects reports whether the name filters keep the rule called name:
// Enable, when non-empty, must list it and Disable must not.
func (s *ruleSelection) selects(name string) bool {
	l := s.loader
	if len(l.Enable) > 0 && !slices.Contains(l.Enable, name) {
		return false
	}
	return !slices.Contains(l.Disable, name)
}

// withinPriority reports whether priority falls inside the loader's bounds.
func (s *ruleSelection) withinPriority(priority int) bool {
	l := s.loader
	if l.MinPriority > 0 && priority > l.MinPriority {
		return false
	}
	return l.MaxPriority <= 0 || priority >= l.MaxPriority
}

func (s *ruleSelection) markLoaded(name string) {
	s.loaded[name] = true
}

// report warns when the name filters leave no rules and, when verbose, lists
// requested names that matched no loaded rule: Enable entries first, then
// Disable entries, each deduplicated.
func (s *ruleSelection) report(sets []*rule.RuleSet) {
	l := s.loader
	if len(l.Enable) == 0 && len(l.Disable) == 0 || l.Warn == nil {
		return
	}
	if countRules(sets) == 0 {
		l.Warn("no rules selected (check --enable/--only/--disable)")
	}
	if !l.Verbose {
		return
	}
	for _, name := range unmatchedNames(append(slices.Clone(l.Enable), l.Disable...), s.loaded) {
		l.Warn(fmt.Sprintf("no rule named %q (check --enable/--only/--disable)", name))
	}
}

// unmatchedNames returns the entries of names not present in matched, keeping
// first-seen order and dropping duplicates.
func unmatchedNames(names []string, matched map[string]bool) []string {
	var out []string
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if matched[n] || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func countRules(sets []*rule.RuleSet) int {
	n := 0
	for _, set := range sets {
		n += len(set.Rules)
	}
	return n
}
