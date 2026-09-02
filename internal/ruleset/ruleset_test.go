package ruleset

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quality-gates/messgo/internal/model"
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
	// Go-specific opinionated rules (ADR-0001 ranks 6-8) also live here.
	goOpinionated := []string{"UncheckedTypeAssertion", "IdenticalBranches", "StructEmbeddingDepth"}

	goSet := loadOne(t, "go")
	for _, name := range moved {
		if ruleByName(goSet, name) != nil {
			t.Errorf("go ruleset should not include %s (it is opinionated, not idiomatic Go)", name)
		}
	}
	for _, name := range goOpinionated {
		if ruleByName(goSet, name) != nil {
			t.Errorf("go ruleset should not include %s (it is opinionated, not default)", name)
		}
	}

	opinionated := loadOne(t, "opinionated")
	for _, name := range moved {
		if ruleByName(opinionated, name) == nil {
			t.Errorf("opinionated ruleset should include %s", name)
		}
	}
	for _, name := range goOpinionated {
		if ruleByName(opinionated, name) == nil {
			t.Errorf("opinionated ruleset should include %s", name)
		}
	}
	want := len(moved) + len(goOpinionated)
	if got := len(opinionated.Rules); got != want {
		t.Errorf("opinionated ruleset has %d rules, want %d", got, want)
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

type loaderThresholdRule struct {
	*rule.Base
	*rule.ThresholdRule
}

func newLoaderThresholdRule() rule.Rule {
	r := &loaderThresholdRule{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property: "limit",
		Default:  10,
		Boundary: rule.AtOrAbove,
		NodeKind: rule.ThresholdFunction,
		FuncMetric: func(_ *rule.Context, fn *model.Function) (rule.ThresholdMeasurement, bool) {
			return rule.ThresholdMeasurement{
				Value: len(fn.Params),
				Args:  []any{string(fn.NodeType()), fn.Name},
			}, true
		},
	})
	return r
}

func TestLoaderConfiguresThresholdRulesAtLoadTime(t *testing.T) {
	rule.Register("Messgo\\Test\\Threshold", newLoaderThresholdRule)
	path := filepath.Join(t.TempDir(), "ruleset.xml")
	xml := `<?xml version="1.0"?>
<ruleset name="test">
  <rule name="LoaderThreshold" message="{0} {1} has value {2} over {3}" class="Messgo\Test\Threshold">
    <properties>
      <property name="limit" value="2"/>
    </properties>
  </rule>
</ruleset>`
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}

	set := loadOne(t, path)
	violations := rule.Analyze(thresholdLoaderFile(2), []*rule.RuleSet{set})

	if len(violations) != 1 {
		t.Fatalf("expected one violation from loaded threshold config, got %d", len(violations))
	}
	if got, want := violations[0].Description, "function sample has value 2 over 2"; got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
}

func TestLoaderRejectsInvalidThresholdConfig(t *testing.T) {
	rule.Register("Messgo\\Test\\InvalidThreshold", newLoaderThresholdRule)
	path := filepath.Join(t.TempDir(), "ruleset.xml")
	xml := `<?xml version="1.0"?>
<ruleset name="test">
  <rule name="LoaderThreshold" message="{0} {1} has value {2} over {3}" class="Messgo\Test\InvalidThreshold">
    <properties>
      <property name="limit" value="bogus"/>
    </properties>
  </rule>
</ruleset>`
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := (&Loader{}).Load(path); err == nil {
		t.Fatal("expected invalid threshold config to fail at load time")
	}
}

func TestNestedGoRefImportsRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "team.xml")
	xml := `<ruleset name="team">
  <rule ref="go">
    <exclude name="DevelopmentCodeFragment" />
  </rule>
</ruleset>
`
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	set := loadOne(t, path)
	if ruleByName(set, "CyclomaticComplexity") == nil {
		t.Fatalf(`ref="go" imported %d rules; expected CyclomaticComplexity`, len(set.Rules))
	}
	if ruleByName(set, "DevelopmentCodeFragment") != nil {
		t.Fatal("exclude DevelopmentCodeFragment was ignored")
	}
}

func TestBareRuleNameResolvesBuiltin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "team.xml")
	xml := `<ruleset name="team">
  <rule ref="LongVariable">
    <priority>2</priority>
    <properties>
      <property name="maximum" value="50" />
    </properties>
  </rule>
</ruleset>
`
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	set := loadOne(t, path)
	r := ruleByName(set, "LongVariable")
	if r == nil {
		t.Fatal(`<rule ref="LongVariable"> imported nothing`)
	}
	if got := rule.BaseOf(r).RulePrio; got != 2 {
		t.Errorf("priority = %d, want 2", got)
	}
	if got := rule.BaseOf(r).RuleProps.Int("maximum", 0); got != 50 {
		t.Errorf("maximum = %d, want 50", got)
	}
}

func TestNestedSingleRuleRefThroughGo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "team.xml")
	xml := `<ruleset name="team">
  <rule ref="go/CyclomaticComplexity"/>
</ruleset>
`
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	set := loadOne(t, path)
	if ruleByName(set, "CyclomaticComplexity") == nil {
		t.Fatal("go/CyclomaticComplexity did not import the rule")
	}
	if ruleByName(set, "LongVariable") != nil {
		t.Fatal("go/CyclomaticComplexity imported an unrelated rule")
	}
}

func TestNestedRelativeFileRefs(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	leaf := filepath.Join(sub, "leaf.xml")
	mid := filepath.Join(dir, "mid.xml")
	team := filepath.Join(dir, "team.xml")
	if err := os.WriteFile(leaf, []byte(`<ruleset name="leaf">
  <rule name="CyclomaticComplexity" class="PHPMD\Rule\CyclomaticComplexity"/>
</ruleset>
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mid, []byte(`<ruleset name="mid">
  <rule ref="sub/leaf.xml"/>
</ruleset>
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(team, []byte(`<ruleset name="team">
  <rule ref="mid.xml"/>
</ruleset>
`), 0o644); err != nil {
		t.Fatal(err)
	}
	set := loadOne(t, team)
	if ruleByName(set, "CyclomaticComplexity") == nil {
		t.Fatal("nested relative file refs did not import CyclomaticComplexity")
	}
}

func TestUnknownSingleRuleRefErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "team.xml")
	xml := `<ruleset name="team">
  <rule ref="naming/ShortVariabl"/>
</ruleset>
`
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Loader{}).Load(path); err == nil {
		t.Fatal("misspelled single-rule ref loaded with no error")
	}
}

func TestRelativeFileRefResolvesAgainstRulesetDir(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.xml")
	team := filepath.Join(dir, "team.xml")
	if err := os.WriteFile(base, []byte(`<ruleset name="base">
  <rule name="CyclomaticComplexity" class="PHPMD\Rule\CyclomaticComplexity"/>
</ruleset>
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(team, []byte(`<ruleset name="team">
  <rule ref="base.xml"/>
</ruleset>
`), 0o644); err != nil {
		t.Fatal(err)
	}
	set := loadOne(t, team)
	if ruleByName(set, "CyclomaticComplexity") == nil {
		t.Fatal("relative ref did not import CyclomaticComplexity")
	}
}

func TestDirectRulesetReferenceCycleReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "self.xml")
	if err := os.WriteFile(path, []byte(`<ruleset name="self">
  <rule ref="self.xml"/>
</ruleset>
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (&Loader{}).Load(path)
	if err == nil {
		t.Fatal("expected cyclic ruleset reference to fail")
	}
	if !strings.Contains(err.Error(), "cyclic ruleset reference") {
		t.Fatalf("error = %q, want cyclic ruleset reference", err)
	}
}

func TestIndirectRulesetReferenceCycleReturnsError(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.xml")
	second := filepath.Join(dir, "second.xml")
	if err := os.WriteFile(first, []byte(`<ruleset name="first">
  <rule ref="second.xml"/>
</ruleset>
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(`<ruleset name="second">
  <rule ref="first.xml"/>
</ruleset>
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, loadErr := (&Loader{}).Load(first)
	if loadErr == nil {
		t.Fatal("expected cyclic ruleset reference to fail")
	}
	canonicalFirst, err := filepath.EvalSymlinks(first)
	if err != nil {
		t.Fatal(err)
	}
	canonicalSecond, err := filepath.EvalSymlinks(second)
	if err != nil {
		t.Fatal(err)
	}
	wantCycle := strings.Join([]string{canonicalFirst, canonicalSecond, canonicalFirst}, " -> ")
	if !strings.Contains(loadErr.Error(), wantCycle) {
		t.Fatalf("error = %q, want cycle %q", loadErr, wantCycle)
	}
}

func TestBareRuleOwnersReuseSessionSources(t *testing.T) {
	session := &loadSession{
		loader:  &Loader{},
		sources: make(map[string]xmlRuleSet),
	}
	base, ruleName := session.resolveRef("CyclomaticComplexity", "")
	if base != "codesize" || ruleName != "CyclomaticComplexity" {
		t.Fatalf("resolved to %q/%q, want codesize/CyclomaticComplexity", base, ruleName)
	}
	sourceCount := len(session.sources)
	ownerCount := len(session.builtinOwners)

	base, ruleName = session.resolveRef("UnusedLocalVariable", "")
	if base != "unusedcode" || ruleName != "UnusedLocalVariable" {
		t.Fatalf("resolved to %q/%q, want unusedcode/UnusedLocalVariable", base, ruleName)
	}
	if len(session.sources) != sourceCount || len(session.builtinOwners) != ownerCount {
		t.Fatalf("second bare-rule lookup rebuilt owner index: sources %d -> %d, owners %d -> %d",
			sourceCount, len(session.sources), ownerCount, len(session.builtinOwners))
	}
}

func TestExpansionKeyIncludesSortedContext(t *testing.T) {
	priority := 2
	alpha := "first"
	zulu := "last"
	override := &xmlRule{
		Priority: &priority,
		Properties: xmlProperties{Property: []xmlProperty{
			{Name: "zulu", Value: &zulu},
			{Name: "alpha", Value: &alpha},
		}},
	}
	excluded := map[string]bool{"zulu": true, "ignored": false, "alpha": true}
	context := expansionKey("location", "Sample", excluded, override)
	want := `"location"|"Sample"|exclude:"alpha"|exclude:"zulu"|priority:2|property:"alpha"="first"|property:"zulu"="last"`
	if context != want {
		t.Fatalf("expansion key = %q, want %q", context, want)
	}
	withoutOverride := `"location"|""|exclude:"alpha"|exclude:"zulu"`
	if got := expansionKey("location", "", excluded, override); got != withoutOverride {
		t.Fatalf("unnamed expansion key = %q, want %q", got, withoutOverride)
	}
	if got := expansionKey("location", "Sample", excluded, nil); got != strings.Replace(withoutOverride, `|""|`, `|"Sample"|`, 1) {
		t.Fatalf("nil-override expansion key = %q", got)
	}
}

func TestRefExpanderAppliesPriorityBoundaries(t *testing.T) {
	cases := []struct {
		name          string
		loader        Loader
		priority      int
		wantRuleCount int
	}{
		{name: "unbounded", priority: 3, wantRuleCount: 1},
		{name: "minimum boundary included", loader: Loader{MinPriority: 3}, priority: 3, wantRuleCount: 1},
		{name: "below minimum importance excluded", loader: Loader{MinPriority: 3}, priority: 4},
		{name: "maximum boundary included", loader: Loader{MaxPriority: 3}, priority: 3, wantRuleCount: 1},
		{name: "above maximum importance excluded", loader: Loader{MaxPriority: 3}, priority: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := &rule.RuleSet{}
			expander := newRefExpander(&loadSession{loader: &tc.loader}, set)
			base := rule.NewBase()
			base.RulePrio = tc.priority
			expander.appendRule(base)
			if len(set.Rules) != tc.wantRuleCount {
				t.Fatalf("rule count = %d, want %d", len(set.Rules), tc.wantRuleCount)
			}
		})
	}
}

func TestRefExpanderBuildsCompleteRuleMetadata(t *testing.T) {
	const class = "Messgo\\Test\\Metadata"
	rule.Register(class, func() rule.Rule { return rule.NewBase() })
	expander := newRefExpander(&loadSession{loader: &Loader{}}, &rule.RuleSet{})
	definition := xmlRule{
		Name:            "Metadata",
		Message:         " message ",
		Class:           class,
		ExternalInfoURL: "https://example.test/rule",
		Since:           "1.2.3",
		Description:     " description ",
	}
	built, err := expander.buildRule("sample-set", definition, &definition)
	if err != nil {
		t.Fatal(err)
	}
	base := rule.BaseOf(built)
	if base.RuleName != "Metadata" || base.RuleMessage != "message" || base.RuleSet != "sample-set" ||
		base.RuleURL != "https://example.test/rule" || base.RuleSince != "1.2.3" ||
		base.RuleDesc != "description" || base.RulePrio != 3 {
		t.Fatalf("built metadata = %+v", base)
	}
}

func TestLoadSessionCachesDecodedSources(t *testing.T) {
	session := &loadSession{sources: make(map[string]xmlRuleSet)}
	first, key, err := session.decode([]byte(`<ruleset name="first"/>`), "codesize")
	if err != nil {
		t.Fatal(err)
	}
	if key != "builtin:codesize" || first.Name != "first" {
		t.Fatalf("first decode = %#v at %q", first, key)
	}
	second, secondKey, err := session.decode([]byte(`not xml`), "codesize")
	if err != nil || secondKey != key || second.Name != first.Name {
		t.Fatalf("cached decode = %#v at %q, error %v", second, secondKey, err)
	}
	if _, _, err := session.decode([]byte(`not xml`), "naming"); err == nil {
		t.Fatal("uncached invalid XML decoded without error")
	}
}

func BenchmarkRepeatedRulesetReferences(b *testing.B) {
	for _, depth := range []int{4, 8, 12} {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			path := repeatedRulesetGraph(b, depth)
			sets, err := (&Loader{}).Load(path)
			if err != nil {
				b.Fatal(err)
			}
			if got := len(sets[0].Rules); got != 1 {
				b.Fatalf("loaded %d rules, want 1", got)
			}
			b.ResetTimer()
			for b.Loop() {
				if _, err := (&Loader{}).Load(path); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func repeatedRulesetGraph(tb testing.TB, depth int) string {
	tb.Helper()
	dir := tb.TempDir()
	previous := "leaf.xml"
	path := filepath.Join(dir, previous)
	if err := os.WriteFile(path, []byte(`<ruleset name="leaf">
  <rule name="CyclomaticComplexity" class="PHPMD\Rule\CyclomaticComplexity"/>
</ruleset>
`), 0o644); err != nil {
		tb.Fatal(err)
	}
	for level := 1; level <= depth; level++ {
		name := fmt.Sprintf("level-%d.xml", level)
		path = filepath.Join(dir, name)
		xml := fmt.Sprintf(`<ruleset name="level-%d">
  <rule ref="%s"/>
  <rule ref="%s"/>
</ruleset>
`, level, previous, previous)
		if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
			tb.Fatal(err)
		}
		previous = name
	}
	return path
}

func thresholdLoaderFile(paramCount int) *model.File {
	fn := &model.Function{Name: "sample", Line: 1, EndLine: 1}
	for range paramCount {
		fn.Params = append(fn.Params, &model.Parameter{})
	}
	file := &model.File{Path: "fixture.go", Package: "fixture", AllFuncs: []*model.Function{fn}}
	fn.File = file
	return file
}
