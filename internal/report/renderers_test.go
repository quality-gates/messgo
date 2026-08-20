package report

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/quality-gates/messgo/internal/rule"
)

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func rendererTestReport() *Report {
	r := &rule.Base{
		RuleName: "Example",
		RulePrio: 2,
		RuleSet:  "ExampleSet",
		RuleURL:  "https://example.test/rule",
		RuleDesc: "example rule",
	}
	return &Report{
		Violations: []*rule.Violation{
			{Rule: r, File: "a.go", BeginLine: 1, EndLine: 2, Description: "first violation", Priority: 2, RuleSetName: "ExampleSet", Package: "sample", Function: "first"},
			{Rule: r, File: "b.go", BeginLine: 4, EndLine: 4, Description: "second violation", Priority: 4, RuleSetName: "ExampleSet", Class: "Sample", Method: "Second"},
		},
		Errors: []ProcessingError{{File: "broken.go", Message: "parse failed"}},
	}
}

func TestCheckedWriterRejectsShortWrites(t *testing.T) {
	w := &checkedWriter{Writer: shortWriter{}}
	n, err := w.Write([]byte("abc"))
	if n != 2 || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("checkedWriter.Write() error = %v, want %v", err, io.ErrShortWrite)
	}
}

func TestCheckedWriterReturnsTheStoredErrorWithoutWritingAgain(t *testing.T) {
	want := errors.New("write failed")
	w := &checkedWriter{Writer: failingWriter{err: want}}
	if n, err := w.Write([]byte("first")); n != 0 || !errors.Is(err, want) {
		t.Fatalf("first Write() = (%d, %v), want (0, %v)", n, err, want)
	}
	if n, err := w.Write([]byte("second")); n != 0 || !errors.Is(err, want) {
		t.Fatalf("second Write() = (%d, %v), want (0, %v)", n, err, want)
	}
}

func renderOutput(t *testing.T, format string) string {
	t.Helper()
	renderer, ok := For(format)
	if !ok {
		t.Fatalf("For(%q) did not return a renderer", format)
	}
	var out strings.Builder
	if err := renderer.Render(&out, rendererTestReport()); err != nil {
		t.Fatalf("%s.Render() error = %v", format, err)
	}
	return out.String()
}

func requireFragments(t *testing.T, output string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(output, fragment) {
			t.Errorf("output does not contain %q:\n%s", fragment, output)
		}
	}
}

func TestRenderersPreserveReportContent(t *testing.T) {
	wantText := "a.go:1  Example  first violation\nb.go:4  Example  second violation\nbroken.go\t-\tparse failed\n"
	if got := renderOutput(t, "text"); got != wantText {
		t.Fatalf("text output = %q, want %q", got, wantText)
	}
	wantANSI := "a.go:1  \x1b[33mExample\x1b[0m  \x1b[31mfirst violation\x1b[0m\nb.go:4  \x1b[33mExample\x1b[0m  \x1b[31msecond violation\x1b[0m\nbroken.go\t-\tparse failed\n"
	if got := renderOutput(t, "ansi"); got != wantANSI {
		t.Fatalf("ansi output = %q, want %q", got, wantANSI)
	}

	requireFragments(t, renderOutput(t, "xml"),
		"<?xml version=\"1.0\" encoding=\"UTF-8\" ?>",
		"<pmd version=", "<file name=\"a.go\">", "beginline=\"1\"", "endline=\"2\"",
		"rule=\"Example\"", "ruleset=\"ExampleSet\"", "package=\"sample\"",
		"externalInfoUrl=\"https://example.test/rule\"", "function=\"first\"", "priority=\"2\"",
		"first violation", "</file>", "<file name=\"b.go\">", "class=\"Sample\"", "method=\"Second\"",
		"priority=\"4\"", "second violation", "<error filename=\"broken.go\" msg=\"parse failed\" />", "</pmd>")
	requireFragments(t, renderOutput(t, "json"),
		`"version":`, `"package": "messgo"`, `"files":`, `"file": "a.go"`, `"beginLine": 1`, `"endLine": 2`,
		`"package": "sample"`, `"function": "first"`, `"class": "Sample"`, `"method": "Second"`,
		`"description": "first violation"`, `"rule": "Example"`, `"ruleSet": "ExampleSet"`,
		`"externalInfoUrl": "https://example.test/rule"`, `"priority": 2`, `"fileName": "broken.go"`, `"message": "parse failed"`)
	if got := renderOutput(t, "github"); got != "::warning file=a.go,line=1,col=1::first violation (Example)\n::warning file=b.go,line=4,col=1::second violation (Example)\n::error file=broken.go::parse failed\n" {
		t.Fatalf("github output = %q", got)
	}
	requireFragments(t, renderOutput(t, "gitlab"),
		`"type": "issue"`, `"check_name": "Example"`, `"description": "first violation"`, `"path": "a.go"`, `"begin": 1`,
		`"description": "second violation"`, `"path": "b.go"`)
	requireFragments(t, renderOutput(t, "checkstyle"),
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?>", "<checkstyle version=", "<file name=\"a.go\">",
		`line="1" column="1" severity="error" message="first violation" source="ExampleSet/Example"/>`,
		`line="4" column="1" severity="info" message="second violation" source="ExampleSet/Example"/>`, "</checkstyle>")
	requireFragments(t, renderOutput(t, "sarif"),
		`"$schema":`, `"version": "2.1.0"`, `"name": "messgo"`, `"id": "Example"`,
		`"helpUri": "https://example.test/rule"`, `"text": "example rule"`, `"ruleId": "Example"`,
		`"uri": "a.go"`, `"startLine": 1`, `"endLine": 2`, `"text": "first violation"`, `"level": "error"`)
	requireFragments(t, renderOutput(t, "html"),
		"<!DOCTYPE html>", "<h1>messgo report</h1>", "<h2>a.go</h2>",
		"<tr><th>Line</th><th>Rule</th><th>Description</th></tr>",
		"<tr><td>1</td><td>Example</td><td>first violation</td></tr>", "<h2>b.go</h2>",
		"<tr><td>4</td><td>Example</td><td>second violation</td></tr>", "</body></html>")
}

func TestRenderersPropagateWriterErrors(t *testing.T) {
	want := errors.New("write failed")
	for _, format := range Formats() {
		t.Run(format+"/error", func(t *testing.T) {
			renderer, ok := For(format)
			if !ok {
				t.Fatalf("For(%q) did not return a renderer", format)
			}
			err := renderer.Render(failingWriter{err: want}, rendererTestReport())
			if !errors.Is(err, want) {
				t.Fatalf("%s.Render() error = %v, want %v", format, err, want)
			}
		})

		t.Run(format+"/success", func(t *testing.T) {
			if output := renderOutput(t, format); output == "" {
				t.Fatalf("%s.Render() produced no output", format)
			}
		})
	}
}
