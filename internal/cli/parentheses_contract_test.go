package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

type parenthesesFinding struct {
	Line        int    `json:"beginLine"`
	Rule        string `json:"rule"`
	Class       string `json:"class"`
	Method      string `json:"method"`
	Function    string `json:"function"`
	Description string `json:"description"`
}

func parenthesesReport(t *testing.T, source, ruleset string) []parenthesesFinding {
	t.Helper()
	dir := writePackage(t, map[string]string{"input.go": source})
	code, out, stderr := runMain(t, dir, "json", ruleset)
	if code != ExitViolation || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q, output = %s; expected findings", code, stderr, out)
	}
	var report struct {
		Files []struct {
			Violations []parenthesesFinding `json:"violations"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	var findings []parenthesesFinding
	for _, file := range report.Files {
		findings = append(findings, file.Violations...)
	}
	return findings
}

func TestEquivalentParenthesesPreserveFindings(t *testing.T) {
	cognitive := writeRulesetFile(t, `<ruleset name="cognitive"><rule ref="rulesets/codesize.xml/CognitiveComplexity"><properties><property name="reportLevel" value="1"/></properties></rule></ruleset>`)
	naming := writeRulesetFile(t, `<ruleset name="naming"><rule ref="rulesets/naming.xml/LongClassName"><properties><property name="maximum" value="2"/></properties></rule></ruleset>`)
	cases := []struct {
		name, source, expression, ruleset, wantRule string
	}{
		{"type declaration", "package p\ntype Thing %s\n", "struct { x int }", naming, "LongClassName"},
		{"receiver", "package p\ntype T struct { x int }\nfunc (t %s) set() { t.x = 1 }\n", "*T", "explicitness-strict", "ImplicitOutput"},
		{"channel range", "package p\nfunc drain(ch chan int) { for range %s {} }\n", "ch", "explicitness", "ImplicitInput"},
		{"stream call", "package p\nimport (\"fmt\"; \"io\")\nfunc write(w io.Writer) { %s(w, \"x\") }\n", "fmt.Fprintf", "explicitness", "ImplicitOutput"},
		{"recursive call", "package p\nfunc recurse() { %s() }\n", "recurse", cognitive, "CognitiveComplexity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := parenthesesReport(t, fmt.Sprintf(tc.source, tc.expression), tc.ruleset)
			if !slices.ContainsFunc(base, func(f parenthesesFinding) bool { return f.Rule == tc.wantRule }) {
				t.Fatalf("baseline missing %s: %+v", tc.wantRule, base)
			}
			for depth := 1; depth <= 3; depth++ {
				expr := strings.Repeat("(", depth) + tc.expression + strings.Repeat(")", depth)
				got := parenthesesReport(t, fmt.Sprintf(tc.source, expr), tc.ruleset)
				if !slices.Equal(base, got) {
					t.Errorf("%s: findings changed\nbase: %+v\n got: %+v", expr, base, got)
				}
			}
		})
	}
}
