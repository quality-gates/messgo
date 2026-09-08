package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func runMain(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Main(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestExitCodeViolation(t *testing.T) {
	path := writeFixture(t, "package p\nfunc f(a, b, c, d, e, f2, g, h, i, j, k int) {}\n")
	code, out, _ := runMain(t, path, "text", "codesize")
	if code != ExitViolation {
		t.Errorf("exit = %d, want %d", code, ExitViolation)
	}
	if !strings.Contains(out, "ExcessiveParameterList") {
		t.Errorf("missing violation in output: %q", out)
	}
}

func TestExitCodeClean(t *testing.T) {
	path := writeFixture(t, "package p\nfunc f(a int) int { return a }\n")
	code, out, _ := runMain(t, path, "text", "codesize")
	if code != ExitSuccess {
		t.Errorf("exit = %d, want %d", code, ExitSuccess)
	}
	if out != "" {
		t.Errorf("expected empty output, got %q", out)
	}
}

func TestIgnoreViolationsOnExit(t *testing.T) {
	path := writeFixture(t, "package p\nfunc f(a, b, c, d, e, f2, g, h, i, j, k int) {}\n")
	code, _, _ := runMain(t, path, "text", "codesize", "--ignore-violations-on-exit")
	if code != ExitSuccess {
		t.Errorf("exit = %d, want %d", code, ExitSuccess)
	}
}

func TestUnknownFormat(t *testing.T) {
	path := writeFixture(t, "package p\n")
	code, _, errOut := runMain(t, path, "bogus", "codesize")
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut, "unknown report format") {
		t.Errorf("missing error message: %q", errOut)
	}
}

func TestVersion(t *testing.T) {
	code, out, _ := runMain(t, "--version")
	if code != ExitSuccess || out != "messgo dev\n" {
		t.Errorf("version: code=%d out=%q", code, out)
	}
}

func TestJSONFormat(t *testing.T) {
	path := writeFixture(t, "package p\nfunc f(a, b, c, d, e, f2, g, h, i, j, k int) {}\n")
	code, out, _ := runMain(t, path, "json", "codesize")
	if code != ExitViolation {
		t.Fatalf("exit = %d", code)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "{") || !strings.Contains(out, "\"rule\": \"ExcessiveParameterList\"") {
		t.Errorf("unexpected json: %q", out)
	}
	if !strings.Contains(out, "\"version\": \"dev\"") {
		t.Errorf("JSON report did not use the development build version: %q", out)
	}
}

// twoViolationFixture trips both ExcessiveParameterList (codesize) and
// ElseExpression (cleancode).
const twoViolationFixture = "package p\n" +
	"func f(a, b, c, d, e, f2, g, h, i, j, k int) int {\n" +
	"\tif a > 0 {\n\t\treturn 1\n\t} else {\n\t\treturn 2\n\t}\n}\n"

func TestEnableOnlySubset(t *testing.T) {
	path := writeFixture(t, twoViolationFixture)
	for _, flag := range []string{"--only", "--enable"} {
		code, out, _ := runMain(t, path, "text", "codesize,cleancode", flag, "ExcessiveParameterList")
		if code != ExitViolation {
			t.Fatalf("%s: exit = %d, want %d", flag, code, ExitViolation)
		}
		if !strings.Contains(out, "ExcessiveParameterList") {
			t.Errorf("%s: expected ExcessiveParameterList in output: %q", flag, out)
		}
		if strings.Contains(out, "ElseExpression") {
			t.Errorf("%s: ElseExpression should be filtered out: %q", flag, out)
		}
	}
}

func TestDisableRule(t *testing.T) {
	path := writeFixture(t, twoViolationFixture)
	code, out, _ := runMain(t, path, "text", "codesize,cleancode", "--disable", "ElseExpression")
	if code != ExitViolation {
		t.Fatalf("exit = %d, want %d", code, ExitViolation)
	}
	if strings.Contains(out, "ElseExpression") {
		t.Errorf("disabled rule still present: %q", out)
	}
	if !strings.Contains(out, "ExcessiveParameterList") {
		t.Errorf("non-disabled rule missing: %q", out)
	}
}

func TestEnableMultipleCommaSeparated(t *testing.T) {
	path := writeFixture(t, twoViolationFixture)
	code, out, _ := runMain(t, path, "text", "codesize,cleancode",
		"--only", "ExcessiveParameterList,ElseExpression")
	if code != ExitViolation {
		t.Fatalf("exit = %d, want %d", code, ExitViolation)
	}
	if !strings.Contains(out, "ExcessiveParameterList") || !strings.Contains(out, "ElseExpression") {
		t.Errorf("both enabled rules should appear: %q", out)
	}
}

func TestMinimumPriorityFilter(t *testing.T) {
	// codesize rules are priority 3; --minimumpriority 2 keeps only prio<=2,
	// so nothing should fire.
	path := writeFixture(t, "package p\nfunc f(a, b, c, d, e, f2, g, h, i, j, k int) {}\n")
	code, _, _ := runMain(t, path, "text", "codesize", "--minimumpriority", "2")
	if code != ExitSuccess {
		t.Errorf("exit = %d, want clean (filtered out)", code)
	}
}

func TestSingleRuleRefPriorityFilterExitClean(t *testing.T) {
	// A custom ruleset referencing a single rule excluded by priority bounds
	// must exit clean (ExitSuccess 0), not abort with an unknown rule error (ExitError 1).
	rulesetPath := filepath.Join(t.TempDir(), "ruleset.xml")
	xml := `<ruleset name="custom">
  <rule ref="codesize/CyclomaticComplexity"/>
</ruleset>
`
	if err := os.WriteFile(rulesetPath, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	srcPath := writeFixture(t, "package main\nfunc main() {}\n")
	code, out, errOut := runMain(t, srcPath, "text", rulesetPath, "--minimumpriority", "1")
	if code != ExitSuccess {
		t.Fatalf("exit = %d, want %d (ExitSuccess); out=%q errOut=%q", code, ExitSuccess, out, errOut)
	}
}

const parseErrorSrc = "package p\nfunc (\n"
const excessiveParamsSrc = "package p\nfunc f(a, b, c, d, e, f2, g, h, i, j, k int) {}\n"

func TestExitCodesMatchPHPMD(t *testing.T) {
	if ExitSuccess != 0 || ExitError != 1 || ExitViolation != 2 || ExitProcessingError != 3 {
		t.Errorf("exit codes = %d/%d/%d/%d, want 0/1/2/3",
			ExitSuccess, ExitError, ExitViolation, ExitProcessingError)
	}
}

func TestExitCodeParseError(t *testing.T) {
	path := writeFixture(t, parseErrorSrc)
	code, _, _ := runMain(t, path, "text", "go")
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
}

func TestIgnoreErrorsOnExitFallsThroughToClean(t *testing.T) {
	path := writeFixture(t, parseErrorSrc)
	code, _, _ := runMain(t, path, "text", "go", "--ignore-errors-on-exit")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
}

func TestParseErrorTakesPrecedenceOverViolation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte(parseErrorSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "viol.go"), []byte(excessiveParamsSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, _ := runMain(t, dir, "text", "codesize")
	if code != 3 {
		t.Errorf("exit = %d, want 3 (processing error precedes violations)", code)
	}
	code, _, _ = runMain(t, dir, "text", "codesize", "--ignore-errors-on-exit")
	if code != 2 {
		t.Errorf("ignore-errors-on-exit: exit = %d, want 2", code)
	}
}

func TestToolErrorsStillExitOne(t *testing.T) {
	path := writeFixture(t, "package p\n")
	code, _, errOut := runMain(t, filepath.Join(t.TempDir(), "missing.go"), "text", "go")
	if code != 1 {
		t.Errorf("missing path: exit = %d, want 1 (%q)", code, errOut)
	}
	code, _, errOut = runMain(t, path, "nope", "go")
	if code != 1 {
		t.Errorf("unknown format: exit = %d, want 1 (%q)", code, errOut)
	}
	code, _, errOut = runMain(t, path, "text", "nope")
	if code != 1 {
		t.Errorf("unknown ruleset: exit = %d, want 1 (%q)", code, errOut)
	}
}

func TestUsageListsProcessingExitCode(t *testing.T) {
	code, out, _ := runMain(t, "--help")
	if code != ExitSuccess {
		t.Fatalf("help exit = %d, want %d", code, ExitSuccess)
	}
	if !strings.Contains(out, "3 = processing error") {
		t.Errorf("usage missing processing-error exit code: %q", out)
	}
}
