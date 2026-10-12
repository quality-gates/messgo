package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// These tests lock the acceptance checks for physical source spans. Expected
// lines and line counts are counted from the fixture text, not from the
// analyzer's own position API.

func TestLineDirectiveReportsPhysicalLocalAndFunctionSpan(t *testing.T) {
	rules := writeRulesetFile(t, methodAndLocalRuleset(3, false))
	for _, directive := range []string{"generated.go:100", "other.go:9999"} {
		t.Run(directive, func(t *testing.T) {
			src := "package p\n//line " + directive + "\nfunc f() {\n\tx := 1\n}\n"
			path := writeFixture(t, src)
			code, out, errOut := runMain(t, path, "json", rules)
			if code != ExitViolation {
				t.Fatalf("exit = %d, want %d, stderr = %s", code, ExitViolation, errOut)
			}
			vs := violationsFor(t, out, path)
			local := requireRule(t, vs, "UnusedLocalVariable")
			if local.BeginLine != 4 || local.EndLine != 4 {
				t.Fatalf("local span = %d-%d, want 4-4", local.BeginLine, local.EndLine)
			}
			method := requireRule(t, vs, "ExcessiveMethodLength")
			if method.BeginLine != 3 || method.EndLine != 5 {
				t.Fatalf("function span = %d-%d, want 3-5", method.BeginLine, method.EndLine)
			}
			const wantDesc = "The function f() has 3 lines of code. Current threshold is set to 3."
			if method.Description != wantDesc {
				t.Fatalf("description = %q, want %q", method.Description, wantDesc)
			}

			_, githubOut, githubErr := runMain(t, path, "github", rules)
			if githubErr != "" {
				t.Fatalf("github stderr = %s", githubErr)
			}
			wantAnnotation := fmt.Sprintf("file=%s,line=4,", path)
			if !strings.Contains(githubOut, wantAnnotation) {
				t.Fatalf("github output = %q, want annotation %q", githubOut, wantAnnotation)
			}
		})
	}
}

func TestLineDirectiveInsideFunctionUsesPhysicalLength(t *testing.T) {
	for _, directive := range []string{"gen.go:1", "elsewhere.go:50"} {
		t.Run(directive, func(t *testing.T) {
			src := "package p\nfunc f() {\n\tx := 1\n\ty := 2\n//line " + directive + "\n\tz := 3\n}\n"
			path := writeFixture(t, src)

			ordinary := writeRulesetFile(t, methodAndLocalRuleset(6, false))
			code, out, errOut := runMain(t, path, "json", ordinary)
			if code != ExitViolation {
				t.Fatalf("ordinary exit = %d, want %d, stderr = %s", code, ExitViolation, errOut)
			}
			vs := violationsFor(t, out, path)
			if got, want := linesFor(vs, "UnusedLocalVariable"), []int{3, 4, 6}; !slices.Equal(got, want) {
				t.Fatalf("local lines = %v, want %v", got, want)
			}
			method := requireRule(t, vs, "ExcessiveMethodLength")
			if method.BeginLine != 2 || method.EndLine != 7 {
				t.Fatalf("function span = %d-%d, want 2-7", method.BeginLine, method.EndLine)
			}
			const wantOrdinary = "The function f() has 6 lines of code. Current threshold is set to 6."
			if method.Description != wantOrdinary {
				t.Fatalf("ordinary description = %q, want %q", method.Description, wantOrdinary)
			}

			spaced := writeRulesetFile(t, methodRuleset(6, true))
			code, out, errOut = runMain(t, path, "json", spaced)
			if code != ExitSuccess {
				t.Fatalf("ignore-whitespace exit = %d, want %d, output = %s, stderr = %s", code, ExitSuccess, out, errOut)
			}

			effective := writeRulesetFile(t, methodRuleset(5, true))
			code, out, errOut = runMain(t, path, "json", effective)
			if code != ExitViolation {
				t.Fatalf("effective exit = %d, want %d, stderr = %s", code, ExitViolation, errOut)
			}
			effectiveMethod := requireRule(t, violationsFor(t, out, path), "ExcessiveMethodLength")
			const wantEffective = "The function f() has 5 lines of code. Current threshold is set to 5."
			if effectiveMethod.Description != wantEffective {
				t.Fatalf("effective description = %q, want %q", effectiveMethod.Description, wantEffective)
			}
		})
	}
}

func TestClassSpanAndCrossFileMethodUsePhysicalLines(t *testing.T) {
	dir := t.TempDir()
	typesPath := filepath.Join(dir, "types.go")
	methodsPath := filepath.Join(dir, "methods.go")
	writeFile(t, typesPath, "package widgets\n//line generated.go:900\ntype Widget struct {\n\tName string\n}\n")
	for _, directive := range []string{"gen.go:1", "other.go:400"} {
		t.Run(directive, func(t *testing.T) {
			writeFile(t, methodsPath, "package widgets\nfunc (w *Widget) Save() int {\n\ta := 1\n//line "+directive+"\n\tb := 2\n\treturn a + b\n}\n")

			ordinary := writeRulesetFile(t, classRuleset(9, false))
			code, out, errOut := runMain(t, dir, "json", ordinary)
			if code != ExitViolation {
				t.Fatalf("ordinary exit = %d, want %d, stderr = %s", code, ExitViolation, errOut)
			}
			class := requireRule(t, violationsFor(t, out, typesPath), "ExcessiveClassLength")
			if class.BeginLine != 3 || class.EndLine != 5 || class.Class != "Widget" {
				t.Fatalf("class span = %d-%d class %q, want 3-5 Widget", class.BeginLine, class.EndLine, class.Class)
			}
			const wantOrdinary = "The class Widget has 9 lines of code. Current threshold is 9."
			if class.Description != wantOrdinary {
				t.Fatalf("ordinary description = %q, want %q", class.Description, wantOrdinary)
			}

			spaced := writeRulesetFile(t, classRuleset(9, true))
			code, out, errOut = runMain(t, dir, "json", spaced)
			if code != ExitSuccess {
				t.Fatalf("ignore-whitespace exit = %d, want %d, output = %s, stderr = %s", code, ExitSuccess, out, errOut)
			}
		})
	}
}

func TestNakedReturnThresholdIgnoresLineDirective(t *testing.T) {
	rules := writeRulesetFile(t, nakedReturnRuleset(6, 100))
	shortPath := writeFixture(t, "package p\nfunc short() (n int) {\n//line huge.go:1000\n\tn = 1\n\treturn\n}\n")
	code, out, errOut := runMain(t, shortPath, "json", rules)
	if code != ExitSuccess {
		t.Fatalf("short function exit = %d, want %d, output = %s, stderr = %s", code, ExitSuccess, out, errOut)
	}

	for _, directive := range []string{"gen.go:1", "other.go:2"} {
		t.Run(directive, func(t *testing.T) {
			src := "package p\nfunc long() (n int) {\n\tn = 1\n\tn = 2\n\tn = 3\n\tn = 4\n//line " + directive + "\n\treturn\n}\n"
			path := writeFixture(t, src)
			code, out, errOut := runMain(t, path, "json", rules)
			if code != ExitViolation {
				t.Fatalf("exit = %d, want %d, stderr = %s", code, ExitViolation, errOut)
			}
			got := requireRule(t, violationsFor(t, out, path), "NakedReturn")
			const want = "The function long() uses a naked return with 8 lines of code and cyclomatic complexity 1."
			if got.Description != want || got.BeginLine != 2 || got.EndLine != 9 {
				t.Fatalf("naked return = %d-%d %q, want 2-9 %q", got.BeginLine, got.EndLine, got.Description, want)
			}
		})
	}
}

func TestNodeFindingsUsePhysicalLines(t *testing.T) {
	assertion := writeFixture(t, "package p\nfunc assert(x any) {\n//line generated.go:50\n\tv := x.(int)\n\tprintln(v)\n}\n")
	code, out, errOut := runMain(t, assertion, "json", "opinionated", "--only", "UncheckedTypeAssertion")
	if code != ExitViolation {
		t.Fatalf("assertion exit = %d, want %d, stderr = %s", code, ExitViolation, errOut)
	}
	got := requireRule(t, violationsFor(t, out, assertion), "UncheckedTypeAssertion")
	if got.BeginLine != 4 {
		t.Fatalf("type assertion line = %d, want 4", got.BeginLine)
	}

	duplicate := writeFixture(t, "package p\nfunc f() {\n\tm := map[string]int{\n\t\t\"a\": 1,\n//line generated.go:80\n\t\t\"a\": 2,\n\t}\n\t_ = m\n}\n")
	code, out, errOut = runMain(t, duplicate, "json", "cleancode", "--only", "DuplicatedArrayKey")
	if code != ExitViolation {
		t.Fatalf("duplicate exit = %d, want %d, stderr = %s", code, ExitViolation, errOut)
	}
	got = requireRule(t, violationsFor(t, out, duplicate), "DuplicatedArrayKey")
	if got.BeginLine != 6 || !strings.Contains(got.Description, "line 4") {
		t.Fatalf("duplicate key = line %d %q, want line 6 first declared at line 4", got.BeginLine, got.Description)
	}

	write := writeFixture(t, "package p\nvar sink int\nfunc touch() {\n//line generated.go:40\n\tsink = 1\n}\n")
	code, out, errOut = runMain(t, write, "json", "explicitness", "--only", "ImplicitOutput")
	if code != ExitViolation {
		t.Fatalf("implicit output exit = %d, want %d, stderr = %s", code, ExitViolation, errOut)
	}
	got = requireRule(t, violationsFor(t, out, write), "ImplicitOutput")
	if got.BeginLine != 5 {
		t.Fatalf("implicit output line = %d, want 5", got.BeginLine)
	}
}

type spanViolation struct {
	BeginLine   int    `json:"beginLine"`
	EndLine     int    `json:"endLine"`
	Class       string `json:"class"`
	Description string `json:"description"`
	Rule        string `json:"rule"`
}

type spanFile struct {
	File       string          `json:"file"`
	Violations []spanViolation `json:"violations"`
}

type spanReport struct {
	Files []spanFile `json:"files"`
}

func violationsFor(t *testing.T, out, path string) []spanViolation {
	t.Helper()
	var rep spanReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}
	for _, file := range rep.Files {
		if file.File == path {
			return file.Violations
		}
	}
	t.Fatalf("report has no file %s:\n%s", path, out)
	return nil
}

func requireRule(t *testing.T, vs []spanViolation, name string) spanViolation {
	t.Helper()
	for _, v := range vs {
		if v.Rule == name {
			return v
		}
	}
	t.Fatalf("missing %s in %+v", name, vs)
	return spanViolation{}
}

func linesFor(vs []spanViolation, name string) []int {
	var lines []int
	for _, v := range vs {
		if v.Rule == name {
			lines = append(lines, v.BeginLine)
		}
	}
	slices.Sort(lines)
	return lines
}

func writeFile(t *testing.T, path, src string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeRulesetFile(t *testing.T, xml string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ruleset.xml")
	writeFile(t, path, xml)
	return path
}

func methodAndLocalRuleset(minimum int, ignoreWhitespace bool) string {
	return fmt.Sprintf(`<ruleset name="span">
  <rule name="ExcessiveMethodLength" message="The {0} {1}() has {2} lines of code. Current threshold is set to {3}." class="PHPMD\Rule\Design\LongMethod">
    <properties>
      <property name="minimum" value="%d"/>
      <property name="ignore-whitespace" value="%t"/>
    </properties>
  </rule>
  <rule name="UnusedLocalVariable" message="Avoid unused local variables such as '{0}'." class="PHPMD\Rule\UnusedLocalVariable"/>
</ruleset>
`, minimum, ignoreWhitespace)
}

func methodRuleset(minimum int, ignoreWhitespace bool) string {
	return fmt.Sprintf(`<ruleset name="span">
  <rule name="ExcessiveMethodLength" message="The {0} {1}() has {2} lines of code. Current threshold is set to {3}." class="PHPMD\Rule\Design\LongMethod">
    <properties>
      <property name="minimum" value="%d"/>
      <property name="ignore-whitespace" value="%t"/>
    </properties>
  </rule>
</ruleset>
`, minimum, ignoreWhitespace)
}

func classRuleset(minimum int, ignoreWhitespace bool) string {
	return fmt.Sprintf(`<ruleset name="span">
  <rule name="ExcessiveClassLength" message="The class {0} has {1} lines of code. Current threshold is {2}." class="PHPMD\Rule\Design\LongClass">
    <properties>
      <property name="minimum" value="%d"/>
      <property name="ignore-whitespace" value="%t"/>
    </properties>
  </rule>
</ruleset>
`, minimum, ignoreWhitespace)
}

func nakedReturnRuleset(minLOC, minCCN int) string {
	return fmt.Sprintf(`<ruleset name="span">
  <rule name="NakedReturn" message="The {0} {1}() uses a naked return with {2} lines of code and cyclomatic complexity {3}." class="messgo\Rule\NakedReturn">
    <properties>
      <property name="minloc" value="%d"/>
      <property name="minccn" value="%d"/>
    </properties>
  </rule>
</ruleset>
`, minLOC, minCCN)
}
