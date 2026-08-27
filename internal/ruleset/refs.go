package ruleset

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/quality-gates/messgo/internal/rule"
)

var leafBuiltinRulesets = []string{
	"cleancode", "codesize", "controversial", "design", "naming", "unusedcode",
}

type refExpander struct {
	session   *loadSession
	set       *rule.RuleSet
	active    []string
	activePos map[string]int
	expanded  map[string]bool
}

func newRefExpander(session *loadSession, set *rule.RuleSet) *refExpander {
	return &refExpander{
		session:   session,
		set:       set,
		activePos: make(map[string]int),
		expanded:  make(map[string]bool),
	}
}

func (e *refExpander) enter(key string) error {
	if pos, active := e.activePos[key]; active {
		cycle := append(append([]string{}, e.active[pos:]...), key)
		return fmt.Errorf("cyclic ruleset reference: %s", strings.Join(cycle, " -> "))
	}
	e.activePos[key] = len(e.active)
	e.active = append(e.active, key)
	return nil
}

func (e *refExpander) leave() {
	last := len(e.active) - 1
	delete(e.activePos, e.active[last])
	e.active = e.active[:last]
}

// addRule handles one <rule> element: either a direct class-based definition
// or a <rule ref="..."> reference to another ruleset (optionally with
// <exclude> children or single-rule property overrides).
func addRule(e *refExpander, setName string, xr xmlRule, fromDir string) error {
	switch {
	case xr.Ref != "":
		return e.addRef(xr, fromDir)
	case xr.Class != "":
		r, err := buildRule(e.session.loader, setName, xr, &xr)
		if err != nil {
			return err
		}
		if r != nil {
			appendRule(e.session.loader, e.set, r)
		}
	}
	return nil
}

func (e *refExpander) addRef(xr xmlRule, fromDir string) error {
	if err := e.expandRef(xr, "", excludeSet(xr.Exclude), &xr, fromDir); err != nil {
		return err
	}
	_, ruleName := resolveRef(xr.Ref, fromDir)
	if ruleName != "" && !setHasRule(e.set, ruleName) {
		return fmt.Errorf("unknown rule %q", xr.Ref)
	}
	return nil
}

func (e *refExpander) expandRef(xr xmlRule, wantName string, parentExclude map[string]bool, ov *xmlRule, fromDir string) error {
	base, ruleName := resolveRef(xr.Ref, fromDir)
	if skipFilteredRef(ruleName, wantName) {
		return nil
	}
	ruleName = coalesceRuleName(ruleName, wantName)
	src, key, err := readSource(e.session, base, fromDir)
	if err != nil {
		return err
	}
	if err := e.enter(key); err != nil {
		return err
	}
	defer e.leave()
	excluded := mergeExclude(parentExclude, xr.Exclude)
	expansion := expansionKey(key, ruleName, excluded, ov)
	if e.expanded[expansion] {
		return nil
	}
	if err := e.importSourceRules(src, ruleName, excluded, ov, rulesetDir(key)); err != nil {
		return err
	}
	e.expanded[expansion] = true
	return nil
}

func readSource(session *loadSession, part, fromDir string) (xmlRuleSet, string, error) {
	data, loc, err := readRuleset(part, fromDir)
	if err != nil {
		return xmlRuleSet{}, "", err
	}
	return session.decode(data, loc)
}

func expansionKey(location, ruleName string, excluded map[string]bool, ov *xmlRule) string {
	var key strings.Builder
	fmt.Fprintf(&key, "%q|%q", location, ruleName)
	names := make([]string, 0, len(excluded))
	for name, isExcluded := range excluded {
		if isExcluded {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&key, "|exclude:%q", name)
	}
	if ruleName == "" || ov == nil {
		return key.String()
	}
	if ov.Priority != nil {
		fmt.Fprintf(&key, "|priority:%d", *ov.Priority)
	}
	props := mergeProps(xmlProperties{}, ov.Properties)
	names = names[:0]
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&key, "|property:%q=%q", name, props[name])
	}
	return key.String()
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

func (e *refExpander) importSourceRules(src xmlRuleSet, ruleName string, excluded map[string]bool, ov *xmlRule, fromDir string) error {
	for _, sr := range src.Rules {
		if err := e.expandSourceRule(src.Name, sr, ruleName, excluded, ov, fromDir); err != nil {
			return err
		}
	}
	return nil
}

func (e *refExpander) expandSourceRule(srcName string, sr xmlRule, ruleName string, excluded map[string]bool, ov *xmlRule, fromDir string) error {
	if sr.Ref != "" {
		return e.expandRef(sr, ruleName, excluded, refOverride(sr, ov, ruleName), fromDir)
	}
	if sr.Class == "" || excluded[sr.Name] || (ruleName != "" && sr.Name != ruleName) {
		return nil
	}
	r, err := buildRule(e.session.loader, srcName, sr, refOverride(sr, ov, ruleName))
	if err != nil {
		return err
	}
	if r != nil {
		appendRule(e.session.loader, e.set, r)
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

func resolveRef(ref, fromDir string) (base, ruleName string) {
	if resolvable(ref, fromDir) {
		return ref, ""
	}
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		if left := ref[:i]; resolvable(left, fromDir) {
			return left, ref[i+1:]
		}
	}
	if owner := builtinRuleOwner(ref); owner != "" {
		return owner, ref
	}
	return ref, ""
}

func resolvable(ident, fromDir string) bool {
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
