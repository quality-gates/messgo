package ruleset

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/quality-gates/messgo/internal/rule"
)

var leafBuiltinRulesets = []string{
	"cleancode", "codesize", "controversial", "design", "naming", "unusedcode",
}

func (l *Loader) addRef(set *rule.RuleSet, xr xmlRule, fromDir string) error {
	if err := l.expandRef(set, xr, "", excludeSet(xr.Exclude), &xr, fromDir); err != nil {
		return err
	}
	_, ruleName := l.resolveRef(xr.Ref, fromDir)
	if ruleName != "" && !setHasRule(set, ruleName) {
		return fmt.Errorf("unknown rule %q", xr.Ref)
	}
	return nil
}

func (l *Loader) expandRef(set *rule.RuleSet, xr xmlRule, wantName string, parentExclude map[string]bool, ov *xmlRule, fromDir string) error {
	base, ruleName := l.resolveRef(xr.Ref, fromDir)
	if skipFilteredRef(ruleName, wantName) {
		return nil
	}
	ruleName = coalesceRuleName(ruleName, wantName)
	data, loc, err := l.read(base, fromDir)
	if err != nil {
		return err
	}
	var src xmlRuleSet
	if err := xml.Unmarshal(data, &src); err != nil {
		return err
	}
	return l.importSourceRules(set, src, ruleName, mergeExclude(parentExclude, xr.Exclude), ov, nextRefDir(base, loc))
}

func setHasRule(set *rule.RuleSet, name string) bool {
	for _, r := range set.Rules {
		if r.Name() == name {
			return true
		}
	}
	return false
}

func skipFilteredRef(resolved, want string) bool {
	return want != "" && resolved != "" && resolved != want
}

func coalesceRuleName(resolved, want string) string {
	if resolved != "" {
		return resolved
	}
	return want
}

func nextRefDir(base, loc string) string {
	if _, builtin := builtinNames[base]; builtin {
		return ""
	}
	return filepath.Dir(loc)
}

func (l *Loader) importSourceRules(set *rule.RuleSet, src xmlRuleSet, ruleName string, excluded map[string]bool, ov *xmlRule, fromDir string) error {
	for _, sr := range src.Rules {
		if err := l.expandSourceRule(set, src.Name, sr, ruleName, excluded, ov, fromDir); err != nil {
			return err
		}
	}
	return nil
}

func (l *Loader) expandSourceRule(set *rule.RuleSet, srcName string, sr xmlRule, ruleName string, excluded map[string]bool, ov *xmlRule, fromDir string) error {
	if sr.Ref != "" {
		return l.expandRef(set, sr, ruleName, excluded, refOverride(sr, ov, ruleName), fromDir)
	}
	if sr.Class == "" || excluded[sr.Name] || (ruleName != "" && sr.Name != ruleName) {
		return nil
	}
	r, err := l.buildRule(srcName, sr, refOverride(sr, ov, ruleName))
	if err != nil {
		return err
	}
	if r != nil {
		l.appendRule(set, r)
	}
	return nil
}

func refOverride(sr xmlRule, ov *xmlRule, ruleName string) *xmlRule {
	if ruleName != "" && ov != nil {
		return ov
	}
	return &sr
}

func mergeExclude(parent map[string]bool, extra []xmlExclude) map[string]bool {
	out := map[string]bool{}
	for k, v := range parent {
		if v {
			out[k] = true
		}
	}
	for _, e := range extra {
		out[e.Name] = true
	}
	return out
}

func (l *Loader) resolveRef(ref, fromDir string) (base, ruleName string) {
	if l.resolvable(ref, fromDir) {
		return ref, ""
	}
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		if left := ref[:i]; l.resolvable(left, fromDir) {
			return left, ref[i+1:]
		}
	}
	if owner := builtinRuleOwner(ref); owner != "" {
		return owner, ref
	}
	return ref, ""
}

func (l *Loader) resolvable(ident, fromDir string) bool {
	if _, ok := builtinNames[ident]; ok {
		return true
	}
	_, err := os.Stat(resolvePath(ident, fromDir))
	return err == nil
}

func resolvePath(part, fromDir string) string {
	if fromDir != "" && !filepath.IsAbs(part) {
		return filepath.Join(fromDir, part)
	}
	return part
}

func builtinRuleOwner(name string) string {
	for _, id := range leafBuiltinRulesets {
		if ruleDefinedInBuiltin(id, name) {
			return id
		}
	}
	return ""
}

func ruleDefinedInBuiltin(id, name string) bool {
	data, err := builtinFS.ReadFile(builtinNames[id])
	if err != nil {
		return false
	}
	var src xmlRuleSet
	if xml.Unmarshal(data, &src) != nil {
		return false
	}
	for _, r := range src.Rules {
		if r.Class != "" && r.Name == name {
			return true
		}
	}
	return false
}
