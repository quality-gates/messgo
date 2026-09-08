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

func TestEmptyPathsIsUsageError(t *testing.T) {
	for _, paths := range []string{"", ",", " , "} {
		code, out, errOut := runMain(t, paths, "text", "go")
		if code != ExitError {
			t.Errorf("paths %q: exit = %d, want %d", paths, code, ExitError)
		}
		if out != "" {
			t.Errorf("paths %q: expected empty stdout, got %q", paths, out)
		}
		if !strings.Contains(errOut, "path") {
			t.Errorf("paths %q: stderr should mention paths, got %q", paths, errOut)
		}
	}
}

func TestEmptyRulesetIsUsageError(t *testing.T) {
	for _, rulesets := range []string{"", ",", " , "} {
		code, out, errOut := runMain(t, "a.go", "text", rulesets)
		if code != ExitError {
			t.Errorf("rulesets %q: exit = %d, want %d", rulesets, code, ExitError)
		}
		if out != "" {
			t.Errorf("rulesets %q: expected empty stdout, got %q", rulesets, out)
		}
		if !strings.Contains(errOut, "ruleset") {
			t.Errorf("rulesets %q: stderr should mention rulesets, got %q", rulesets, errOut)
		}
	}
}

func TestBlankEntriesInListsAreDropped(t *testing.T) {
	a := writeFixture(t, excessiveParamsSrc)
	b := writeFixture(t, "package p\nfunc g() {}\n")
	codeBlank, outBlank, errBlank := runMain(t, a+",,"+b, "text", "go,")
	codeClean, outClean, errClean := runMain(t, a+","+b, "text", "go")
	if codeBlank != codeClean {
		t.Errorf("exit = %d, want %d", codeBlank, codeClean)
	}
	if outBlank != outClean {
		t.Errorf("stdout mismatch:\nblank %q\nclean %q", outBlank, outClean)
	}
	if errBlank != errClean {
		t.Errorf("stderr mismatch:\nblank %q\nclean %q", errBlank, errClean)
	}
}

// --- issue #78: malformed option values must not be silently accepted ---

func TestInvalidPriorityValue(t *testing.T) {
	path := writeFixture(t, "package p\nfunc f(a int) int { return a }\n")
	cases := []struct{ flag, value string }{
		{"--minimumpriority", "abc"},
		{"--minimumpriority", "-1"},
		{"--minimumpriority", "1.5"},
		{"--minimumpriority", ""},
		{"--maximumpriority", "abc"},
		{"--maximumpriority", "-1"},
	}
	for _, tc := range cases {
		code, out, errOut := runMain(t, path, "text", "codesize", tc.flag, tc.value)
		if code != ExitError {
			t.Errorf("%s %q: exit = %d, want %d", tc.flag, tc.value, code, ExitError)
		}
		if !strings.Contains(errOut, "invalid value for "+tc.flag) {
			t.Errorf("%s %q: stderr should name the flag and the value, got %q", tc.flag, tc.value, errOut)
		}
		if out != "" {
			t.Errorf("%s %q: expected empty stdout, got %q", tc.flag, tc.value, out)
		}
	}
}

func TestValidPriorityValuesStillWork(t *testing.T) {
	path := writeFixture(t, excessiveParamsSrc)
	code, _, errOut := runMain(t, path, "text", "codesize", "--minimumpriority", "0")
	if code != ExitViolation {
		t.Errorf("--minimumpriority 0: exit = %d, want %d (%q)", code, ExitViolation, errOut)
	}
	code, _, _ = runMain(t, path, "text", "codesize", "--minimumpriority", "+3")
	if code != ExitViolation {
		t.Errorf("--minimumpriority +3: exit = %d, want %d", code, ExitViolation)
	}
}

func TestValueFlagMissingValue(t *testing.T) {
	path := writeFixture(t, excessiveParamsSrc)
	for _, flag := range []string{
		"--reportfile", "--suffixes", "--exclude", "--enable", "--only", "--disable",
		"--minimumpriority", "--maximumpriority",
	} {
		code, out, errOut := runMain(t, path, "text", "codesize", flag)
		if code != ExitError {
			t.Errorf("%s (trailing): exit = %d, want %d", flag, code, ExitError)
		}
		if !strings.Contains(errOut, flag+" requires a value") {
			t.Errorf("%s (trailing): stderr should say the flag requires a value, got %q", flag, errOut)
		}
		if out != "" {
			t.Errorf("%s (trailing): expected empty stdout, got %q", flag, out)
		}
	}
	// A following option token is not a value.
	code, _, errOut := runMain(t, path, "text", "codesize", "--enable", "--verbose")
	if code != ExitError {
		t.Errorf("--enable followed by option: exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut, "--enable requires a value") {
		t.Errorf("--enable followed by option: got %q", errOut)
	}
}

func TestEnableSelectingNoRulesWarns(t *testing.T) {
	path := writeFixture(t, excessiveParamsSrc)
	code, out, errOut := runMain(t, path, "text", "codesize", "--enable", "Nope")
	if code != ExitSuccess {
		t.Errorf("exit = %d, want %d", code, ExitSuccess)
	}
	if out != "" {
		t.Errorf("expected empty stdout, got %q", out)
	}
	if !strings.Contains(errOut, "no rules selected") {
		t.Errorf("stderr should warn about no rules selected, got %q", errOut)
	}
	// Same warning when the whitelist is fully cancelled by the blacklist.
	_, out, errOut = runMain(t, path, "text", "codesize", "--enable", "ExcessiveParameterList", "--disable", "ExcessiveParameterList")
	if out != "" {
		t.Errorf("expected empty stdout, got %q", out)
	}
	if !strings.Contains(errOut, "no rules selected") {
		t.Errorf("stderr should warn about no rules selected, got %q", errOut)
	}
}

func TestVerboseWarnsUnmatchedFilterNames(t *testing.T) {
	gotoSrc := "package p\nfunc f() {\n\tgoto end\nend:\n}\n"
	path := writeFixture(t, gotoSrc)
	code, out, errOut := runMain(t, path, "text", "design", "--enable", "GotoStatement,Nope", "--verbose")
	if code != ExitViolation {
		t.Fatalf("exit = %d, want %d (out=%q err=%q)", code, ExitViolation, out, errOut)
	}
	if !strings.Contains(out, "GotoStatement") {
		t.Errorf("matched rule should still fire: %q", out)
	}
	if !strings.Contains(errOut, "Nope") {
		t.Errorf("verbose stderr should name the unmatched filter entry, got %q", errOut)
	}
}

func TestInfoFlagsAnywhere(t *testing.T) {
	path := writeFixture(t, excessiveParamsSrc)
	code, out, errOut := runMain(t, path, "text", "codesize", "--help")
	if code != ExitSuccess || errOut != "" {
		t.Errorf("--help after positionals: code=%d err=%q", code, errOut)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("--help after positionals should print usage, got %q", out)
	}
	code, out, _ = runMain(t, path, "text", "codesize", "--version")
	if code != ExitSuccess || !strings.Contains(out, "messgo") {
		t.Errorf("--version after positionals: code=%d out=%q", code, out)
	}
	code, out, _ = runMain(t, path, "--help", "text", "codesize")
	if code != ExitSuccess || !strings.Contains(out, "Usage:") {
		t.Errorf("--help between positionals: code=%d out=%q", code, out)
	}
}

func TestSurplusPositional(t *testing.T) {
	path := writeFixture(t, excessiveParamsSrc)
	code, out, errOut := runMain(t, path, "text", "codesize", "extra")
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut, "extra") {
		t.Errorf("stderr should mention the surplus argument, got %q", errOut)
	}
	if out != "" {
		t.Errorf("expected empty stdout, got %q", out)
	}
}
