// Package ruleset loads PHPMD-format ruleset XML files into runnable RuleSets.
// The XML schema and built-in rulesets mirror PHPMD exactly; the rule classes
// they reference are implemented in internal/rules and registered by class
// name.
package ruleset

import (
	"embed"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/quality-gates/messgo/internal/rule"

	// Register all rule implementations.
	_ "github.com/quality-gates/messgo/internal/rules/cleancode"
	_ "github.com/quality-gates/messgo/internal/rules/codesize"
	_ "github.com/quality-gates/messgo/internal/rules/controversial"
	_ "github.com/quality-gates/messgo/internal/rules/design"
	_ "github.com/quality-gates/messgo/internal/rules/naming"
	_ "github.com/quality-gates/messgo/internal/rules/opinionated"
	_ "github.com/quality-gates/messgo/internal/rules/unusedcode"
)

//go:embed builtin/*.xml
var builtinFS embed.FS

// builtinNames maps the short ruleset identifiers accepted on the command line
// to the embedded XML file.
var builtinNames = map[string]string{
	"cleancode":     "builtin/cleancode.xml",
	"codesize":      "builtin/codesize.xml",
	"controversial": "builtin/controversial.xml",
	"design":        "builtin/design.xml",
	"naming":        "builtin/naming.xml",
	"unusedcode":    "builtin/unusedcode.xml",
	"go":            "builtin/go.xml",
	"opinionated":   "builtin/opinionated.xml",
}

// BuiltinNames returns the sorted list of built-in ruleset identifiers.
func BuiltinNames() []string {
	return []string{"cleancode", "codesize", "controversial", "design", "go", "naming", "opinionated", "unusedcode"}
}

// xml structures -----------------------------------------------------------

type xmlRuleSet struct {
	XMLName     xml.Name  `xml:"ruleset"`
	Name        string    `xml:"name,attr"`
	Description string    `xml:"description"`
	Rules       []xmlRule `xml:"rule"`
}

type xmlRule struct {
	Name            string        `xml:"name,attr"`
	Message         string        `xml:"message,attr"`
	Class           string        `xml:"class,attr"`
	Ref             string        `xml:"ref,attr"`
	ExternalInfoURL string        `xml:"externalInfoUrl,attr"`
	Since           string        `xml:"since,attr"`
	Description     string        `xml:"description"`
	Priority        *int          `xml:"priority"`
	Properties      xmlProperties `xml:"properties"`
	Exclude         []xmlExclude  `xml:"exclude"`
}

type xmlExclude struct {
	Name string `xml:"name,attr"`
}

type xmlProperties struct {
	Property []xmlProperty `xml:"property"`
}

type xmlProperty struct {
	Name  string  `xml:"name,attr"`
	Value *string `xml:"value,attr"`
	// Inline value form: <property name="x"><value>...</value></property>
	InnerValue string `xml:"value"`
}

// Loader builds RuleSets, applying optional rule filters.
type Loader struct {
	// MinPriority drops rules with a numerically larger priority value (lower
	// importance), mirroring PHPMD's --minimumpriority. Zero means no limit.
	MinPriority int
	// MaxPriority drops rules with a numerically smaller priority value (higher
	// importance), mirroring PHPMD's --maximumpriority. Zero means no limit.
	MaxPriority int
	// Warn receives messages about skipped/unknown rules and unknown properties.
	Warn func(string)
	// Verbose enables diagnostics for skipped unimplemented rules.
	Verbose bool
}

// Load resolves a comma-separated list of ruleset identifiers or file paths
// into RuleSets.
func (l *Loader) Load(spec string) ([]*rule.RuleSet, error) {
	session := &loadSession{
		loader:     l,
		sources:    make(map[string]xmlRuleSet),
		candidates: make(map[*rule.RuleSet][]ruleCandidate),
	}
	var sets []*rule.RuleSet
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		data, loc, err := readRuleset(part, "")
		if err != nil {
			return nil, err
		}
		set, err := session.parse(data, loc)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", part, err)
		}
		sets = append(sets, set)
	}
	if err := dedupeRules(sets, session.candidates); err != nil {
		return nil, err
	}
	return sets, nil
}

type loadSession struct {
	loader     *Loader
	sources    map[string]xmlRuleSet
	candidates map[*rule.RuleSet][]ruleCandidate

	builtinOwners map[string]string
}

type candidateKind uint8

const (
	candidateInherited candidateKind = iota
	candidateDefinition
	candidateOverride
)

type ruleCandidate struct {
	rule  rule.Rule
	class string
	kind  candidateKind
}

type ruleSlot struct {
	setIndex  int
	ruleIndex int
}

type selectedCandidate struct {
	candidate ruleCandidate
	slot      ruleSlot
}

func builtinRuleOwner(session *loadSession, name string) string {
	if session.builtinOwners == nil {
		session.builtinOwners = make(map[string]string)
		for _, id := range leafBuiltinRulesets {
			source, _, err := readSource(session, id, "")
			if err != nil {
				continue
			}
			for _, candidate := range source.Rules {
				if candidate.Class != "" {
					if _, exists := session.builtinOwners[candidate.Name]; !exists {
						session.builtinOwners[candidate.Name] = id
					}
				}
			}
		}
	}
	return session.builtinOwners[name]
}

// FilterRules narrows the loaded rule sets by rule name, in place. When enable
// is non-empty, only rules whose name appears in it are kept (a whitelist —
// the CLI's --enable/--only). Any rule whose name appears in disable is then
// removed (a blacklist — the CLI's --disable). Names are matched exactly and
// case-sensitively; names that match no loaded rule are simply ignored. An
// empty enable list means "keep everything" before disable is applied.
func FilterRules(sets []*rule.RuleSet, enable, disable []string) {
	ApplyRuleFilter(sets, enable, disable)
}

// FilterResult reports the outcome of a name-based rule filter.
type FilterResult struct {
	// Remaining is the number of rules kept after filtering.
	Remaining int
	// Unmatched lists requested names that matched no loaded rule, in the
	// order given: --enable/--only entries first, then --disable entries,
	// each deduplicated.
	Unmatched []string
}

// ApplyRuleFilter applies the same filtering as FilterRules and additionally
// reports how many rules survived and which requested names matched nothing.
func ApplyRuleFilter(sets []*rule.RuleSet, enable, disable []string) FilterResult {
	if len(enable) == 0 && len(disable) == 0 {
		return FilterResult{Remaining: countRules(sets)}
	}
	matched := make(map[string]bool)
	for _, set := range sets {
		for _, r := range set.Rules {
			matched[r.Name()] = true
		}
	}
	res := FilterResult{Unmatched: unmatchedNames(append(append([]string{}, enable...), disable...), matched)}
	enabled := toSet(enable)
	disabled := toSet(disable)
	for _, set := range sets {
		set.Rules = filterSet(set.Rules, enabled, disabled)
		res.Remaining += len(set.Rules)
	}
	return res
}

// filterSet keeps the rules of set that survive the enable whitelist and the
// disable blacklist, in place.
func filterSet(rules []rule.Rule, enabled, disabled map[string]bool) []rule.Rule {
	kept := rules[:0]
	for _, r := range rules {
		name := r.Name()
		if len(enabled) > 0 && !enabled[name] {
			continue
		}
		if disabled[name] {
			continue
		}
		kept = append(kept, r)
	}
	return kept
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

func toSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

// dedupeRules merges same-named candidates across the loaded sets. Identical
// inherited rules are collapsed, while an explicit single-rule override wins
// over an inherited rule regardless of declaration order. Distinct direct
// definitions are rejected instead of silently selecting one.
func dedupeRules(sets []*rule.RuleSet, candidatesBySet map[*rule.RuleSet][]ruleCandidate) error {
	selected, err := selectCandidates(sets, candidatesBySet)
	if err != nil {
		return err
	}
	winners := selectedWinners(selected)
	for setIndex, set := range sets {
		set.Rules = keepWinners(set, setIndex, winners)
	}
	return nil
}

func selectCandidates(sets []*rule.RuleSet, candidatesBySet map[*rule.RuleSet][]ruleCandidate) (map[string]selectedCandidate, error) {
	selected := map[string]selectedCandidate{}
	for setIndex, set := range sets {
		candidates := candidatesFor(set, candidatesBySet)
		for ruleIndex, incoming := range candidates {
			if err := selectCandidate(selected, incoming, ruleSlot{setIndex: setIndex, ruleIndex: ruleIndex}); err != nil {
				return nil, err
			}
		}
	}
	return selected, nil
}

func candidatesFor(set *rule.RuleSet, candidatesBySet map[*rule.RuleSet][]ruleCandidate) []ruleCandidate {
	candidates := candidatesBySet[set]
	if len(candidates) == len(set.Rules) {
		return candidates
	}
	return fallbackCandidates(set.Rules)
}

func selectCandidate(selected map[string]selectedCandidate, incoming ruleCandidate, slot ruleSlot) error {
	name := incoming.rule.Name()
	current, exists := selected[name]
	if !exists {
		selected[name] = selectedCandidate{candidate: incoming, slot: slot}
		return nil
	}

	winner, replace, err := mergeCandidates(current.candidate, incoming)
	if err != nil {
		return err
	}
	if replace {
		current.candidate = winner
		selected[name] = current
	}
	return nil
}

func selectedWinners(selected map[string]selectedCandidate) map[ruleSlot]rule.Rule {
	winners := map[ruleSlot]rule.Rule{}
	for _, current := range selected {
		winners[current.slot] = current.candidate.rule
	}
	return winners
}

func keepWinners(set *rule.RuleSet, setIndex int, winners map[ruleSlot]rule.Rule) []rule.Rule {
	kept := set.Rules[:0]
	for ruleIndex := range set.Rules {
		if winner, ok := winners[ruleSlot{setIndex: setIndex, ruleIndex: ruleIndex}]; ok {
			kept = append(kept, winner)
		}
	}
	return kept
}

func fallbackCandidates(rules []rule.Rule) []ruleCandidate {
	candidates := make([]ruleCandidate, len(rules))
	for index, r := range rules {
		candidates[index] = ruleCandidate{rule: r}
	}
	return candidates
}

func mergeCandidates(existing, incoming ruleCandidate) (ruleCandidate, bool, error) {
	if sameRuleDefinition(existing, incoming) {
		return existing, false, nil
	}
	if existing.kind == candidateOverride {
		if incoming.kind == candidateOverride {
			return ruleCandidate{}, false, fmt.Errorf("conflicting overrides for rule %q", existing.rule.Name())
		}
		return existing, false, nil
	}
	if incoming.kind == candidateOverride {
		return incoming, true, nil
	}
	if existing.kind == candidateDefinition && incoming.kind == candidateDefinition {
		return ruleCandidate{}, false, fmt.Errorf("conflicting definitions for rule %q", existing.rule.Name())
	}
	return existing, false, nil
}

func sameRuleDefinition(left, right ruleCandidate) bool {
	if left.class != right.class {
		return false
	}
	leftBase := rule.BaseOf(left.rule)
	rightBase := rule.BaseOf(right.rule)
	if leftBase == nil {
		return rightBase == nil
	}
	if rightBase == nil {
		return false
	}
	return sameRuleMetadata(leftBase, rightBase) && sameProperties(leftBase.RuleProps, rightBase.RuleProps)
}

func sameRuleMetadata(left, right *rule.Base) bool {
	return left.RuleName == right.RuleName &&
		left.RuleMessage == right.RuleMessage &&
		left.RulePrio == right.RulePrio &&
		left.RuleURL == right.RuleURL &&
		left.RuleDesc == right.RuleDesc &&
		left.RuleSince == right.RuleSince
}

func sameProperties(left, right rule.Properties) bool {
	if len(left) != len(right) {
		return false
	}
	for name, value := range left {
		if right[name] != value {
			return false
		}
	}
	return true
}

func readRuleset(part, fromDir string) ([]byte, string, error) {
	if file, ok := builtinNames[part]; ok {
		data, err := builtinFS.ReadFile(file)
		return data, part, err
	}
	path := resolvePath(part, fromDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("unknown ruleset or file %q: %w", part, err)
	}
	return data, path, nil
}

func (s *loadSession) parse(data []byte, loc string) (*rule.RuleSet, error) {
	xrs, key, err := s.decode(data, loc)
	if err != nil {
		return nil, err
	}
	set := &rule.RuleSet{
		Name:        xrs.Name,
		Description: strings.TrimSpace(xrs.Description),
	}
	expander := newRefExpander(s, set)
	if err := expander.enter(key); err != nil {
		return nil, err
	}
	defer expander.leave()
	for _, xr := range xrs.Rules {
		if err := addRule(expander, xrs.Name, xr, rulesetDir(key)); err != nil {
			return nil, err
		}
	}
	if s.candidates != nil {
		s.candidates[set] = expander.candidates
	}
	return set, nil
}

func (s *loadSession) decode(data []byte, loc string) (xmlRuleSet, string, error) {
	key, err := canonicalRulesetLocation(loc)
	if err != nil {
		return xmlRuleSet{}, "", err
	}
	if src, ok := s.sources[key]; ok {
		return src, key, nil
	}
	var src xmlRuleSet
	if err := xml.Unmarshal(data, &src); err != nil {
		return xmlRuleSet{}, "", err
	}
	s.sources[key] = src
	return src, key, nil
}

func canonicalRulesetLocation(loc string) (string, error) {
	if _, builtin := builtinNames[loc]; builtin {
		return "builtin:" + loc, nil
	}
	abs, err := filepath.Abs(loc)
	if err != nil {
		return "", fmt.Errorf("resolve ruleset path %q: %w", loc, err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = resolved
	}
	return filepath.Clean(abs), nil
}

func rulesetDir(key string) string {
	if strings.HasPrefix(key, "builtin:") {
		return ""
	}
	return filepath.Dir(key)
}

func excludeSet(excludes []xmlExclude) map[string]bool {
	set := map[string]bool{}
	for _, e := range excludes {
		set[e.Name] = true
	}
	return set
}

// mergeProps reads base properties then applies overrides on top.
func mergeProps(base, override xmlProperties) rule.Properties {
	props := rule.Properties{}
	for _, p := range base.Property {
		if !hasPropValue(p) {
			continue
		}
		props[p.Name] = propValue(p)
	}
	for _, p := range override.Property {
		if !hasPropValue(p) {
			continue
		}
		props[p.Name] = propValue(p)
	}
	return props
}

func hasPropValue(p xmlProperty) bool {
	return p.Value != nil || strings.TrimSpace(p.InnerValue) != ""
}

func propValue(p xmlProperty) string {
	if p.Value != nil {
		return *p.Value
	}
	if p.InnerValue != "" {
		return strings.TrimSpace(p.InnerValue)
	}
	return ""
}
