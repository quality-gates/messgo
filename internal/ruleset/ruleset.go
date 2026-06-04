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
}

// BuiltinNames returns the sorted list of built-in ruleset identifiers.
func BuiltinNames() []string {
	return []string{"cleancode", "codesize", "controversial", "design", "go", "naming", "unusedcode"}
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
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
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
	var sets []*rule.RuleSet
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		data, _, err := l.read(part)
		if err != nil {
			return nil, err
		}
		set, err := l.parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", part, err)
		}
		sets = append(sets, set)
	}
	return sets, nil
}

func (l *Loader) read(part string) ([]byte, string, error) {
	if file, ok := builtinNames[part]; ok {
		data, err := builtinFS.ReadFile(file)
		return data, part, err
	}
	data, err := os.ReadFile(part)
	if err != nil {
		return nil, "", fmt.Errorf("unknown ruleset or file %q: %w", part, err)
	}
	return data, part, nil
}

func (l *Loader) warn(format string, args ...any) {
	if l.Warn != nil {
		l.Warn(fmt.Sprintf(format, args...))
	}
}

func (l *Loader) parse(data []byte) (*rule.RuleSet, error) {
	var xrs xmlRuleSet
	if err := xml.Unmarshal(data, &xrs); err != nil {
		return nil, err
	}
	set := &rule.RuleSet{
		Name:        xrs.Name,
		Description: strings.TrimSpace(xrs.Description),
	}
	for _, xr := range xrs.Rules {
		if err := l.addRule(set, xrs.Name, xr); err != nil {
			return nil, err
		}
	}
	return set, nil
}

// addRule handles one <rule> element: either a direct class-based definition
// or a <rule ref="..."> reference to another ruleset (optionally with
// <exclude> children or single-rule property overrides).
func (l *Loader) addRule(set *rule.RuleSet, setName string, xr xmlRule) error {
	switch {
	case xr.Ref != "":
		return l.addRef(set, xr)
	case xr.Class != "":
		if r := l.buildRule(setName, xr, &xr); r != nil {
			l.appendRule(set, r)
		}
	}
	return nil
}

// addRef expands a <rule ref="..."> element. A ref to a whole ruleset imports
// all its rules minus any <exclude>d names; a ref of the form "ruleset/Rule"
// imports a single rule, applying property/priority overrides from xr.
func (l *Loader) addRef(set *rule.RuleSet, xr xmlRule) error {
	base, ruleName := l.splitRef(xr.Ref)
	data, _, err := l.read(base)
	if err != nil {
		return err
	}
	var src xmlRuleSet
	if err := xml.Unmarshal(data, &src); err != nil {
		return err
	}
	excluded := excludeSet(xr.Exclude)
	for _, sr := range src.Rules {
		l.processRefRule(set, src.Name, sr, ruleName, excluded, &xr)
	}
	return nil
}

func (l *Loader) processRefRule(set *rule.RuleSet, srcName string, sr xmlRule, ruleName string, excluded map[string]bool, xr *xmlRule) {
	if sr.Class == "" {
		return
	}
	if ruleName != "" {
		if sr.Name == ruleName {
			if r := l.buildRule(srcName, sr, xr); r != nil {
				l.appendRule(set, r)
			}
		}
		return
	}
	if excluded[sr.Name] {
		return
	}
	if r := l.buildRule(srcName, sr, &sr); r != nil {
		l.appendRule(set, r)
	}
}

// buildRule constructs a configured rule from a definition (def, which carries
// message/class/url/since/description) and an override source (ov, which
// carries priority and property overrides — usually the same element, but for
// a single-rule ref it is the referencing element).
func (l *Loader) buildRule(setName string, def xmlRule, ov *xmlRule) rule.Rule {
	ctor, ok := rule.Lookup(def.Class)
	if !ok {
		l.warn("skipping unimplemented rule %s (%s)", def.Name, def.Class)
		return nil
	}
	r := ctor()
	base := rule.BaseOf(r)
	if base == nil {
		l.warn("rule %s does not expose metadata", def.Name)
		return nil
	}
	base.RuleName = def.Name
	base.RuleMessage = strings.TrimSpace(def.Message)
	base.RuleSet = setName
	base.RuleURL = def.ExternalInfoURL
	base.RuleSince = def.Since
	base.RuleDesc = strings.TrimSpace(def.Description)
	base.RulePrio = 3
	if def.Priority != nil {
		base.RulePrio = *def.Priority
	}
	base.RuleProps = mergeProps(def.Properties, ov.Properties)
	if ov.Priority != nil {
		base.RulePrio = *ov.Priority
	}
	return r
}

// appendRule adds a rule to the set unless it is filtered out by the
// configured priority bounds.
func (l *Loader) appendRule(set *rule.RuleSet, r rule.Rule) {
	prio := rule.BaseOf(r).RulePrio
	if l.MinPriority > 0 && prio > l.MinPriority {
		return
	}
	if l.MaxPriority > 0 && prio < l.MaxPriority {
		return
	}
	set.Rules = append(set.Rules, r)
}

// splitRef resolves a ref string into a (ruleset, rule) pair. "naming" →
// ("naming", ""); "naming/ShortVariable" → ("naming", "ShortVariable").
func (l *Loader) splitRef(ref string) (base, ruleName string) {
	if l.resolvable(ref) {
		return ref, ""
	}
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		if left := ref[:i]; l.resolvable(left) {
			return left, ref[i+1:]
		}
	}
	return ref, ""
}

// resolvable reports whether ident names a built-in ruleset or an existing file.
func (l *Loader) resolvable(ident string) bool {
	if _, ok := builtinNames[ident]; ok {
		return true
	}
	_, err := os.Stat(ident)
	return err == nil
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
		props[p.Name] = propValue(p)
	}
	for _, p := range override.Property {
		props[p.Name] = propValue(p)
	}
	return props
}

func propValue(p xmlProperty) string {
	if p.Value == "" && p.InnerValue != "" {
		return strings.TrimSpace(p.InnerValue)
	}
	return p.Value
}
