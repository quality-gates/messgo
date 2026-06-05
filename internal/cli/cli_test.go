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
	if code != ExitSuccess || !strings.Contains(out, "messgo") {
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
