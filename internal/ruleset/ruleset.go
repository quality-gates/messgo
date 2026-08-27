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
	// Warn receives messages about skipped/unknown rules.
	Warn func(string)
}

// Load resolves a comma-separated list of ruleset identifiers or file paths
// into RuleSets.
func (l *Loader) Load(spec string) ([]*rule.RuleSet, error) {
	session := &loadSession{
		loader:  l,
		sources: make(map[string]xmlRuleSet),
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
	dedupeRules(sets)
	return sets, nil
}

type loadSession struct {
	loader        *Loader
	sources       map[string]xmlRuleSet
	builtinOwners map[string]string
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
	if len(enable) == 0 && len(disable) == 0 {
		return
	}
	enabled := toSet(enable)
	disabled := toSet(disable)
	for _, set := range sets {
		kept := set.Rules[:0]
		for _, r := range set.Rules {
			name := r.Name()
			if len(enabled) > 0 && !enabled[name] {
				continue
			}
			if disabled[name] {
				continue
			}
			kept = append(kept, r)
		}
		set.Rules = kept
	}
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

// dedupeRules drops rules whose name has already appeared in an earlier set,
// so overlapping ruleset specs (e.g. "go,codesize", where "go" already
// imports "codesize") do not run the same rule twice and emit duplicate
// violations. The first occurrence wins, preserving any tuning the earlier
// ruleset applied.
func dedupeRules(sets []*rule.RuleSet) {
	seen := map[string]bool{}
	for _, set := range sets {
		kept := set.Rules[:0]
		for _, r := range set.Rules {
			name := rule.BaseOf(r).RuleName
			if seen[name] {
				continue
			}
			seen[name] = true
			kept = append(kept, r)
		}
		set.Rules = kept
	}
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
