package report

import (
	"bytes"
	"encoding/json"
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

func TestSARIFEmptyReportUsesArrays(t *testing.T) {
	var buf bytes.Buffer
	if err := (SARIFRenderer{}).Render(&buf, &Report{}); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	run := doc["runs"].([]any)[0].(map[string]any)
	if run["results"] == nil {
		t.Fatalf("SARIF results is null\n%s", buf.String())
	}
	driver := run["tool"].(map[string]any)["driver"].(map[string]any)
	if driver["rules"] == nil {
		t.Fatalf("SARIF rules is null\n%s", buf.String())
	}
}

func TestSARIFErrorOutputOmitsRegion(t *testing.T) {
	var buf bytes.Buffer
	r := &rule.Base{RuleName: "Stub"}
	rep := &Report{
		Errors: []ProcessingError{
			{File: "broken.go", Message: "syntax error"},
		},
		Violations: []*rule.Violation{
			{File: "zero_line.go", BeginLine: 0, Rule: r},
			{File: "valid_line.go", BeginLine: 5, EndLine: 6, Rule: r},
		},
	}
	if err := (SARIFRenderer{}).Render(&buf, rep); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["$schema"] != "https://json.schemastore.org/sarif-2.1.0.json" {
		t.Fatalf("SARIF $schema = %v, want https://json.schemastore.org/sarif-2.1.0.json", doc["$schema"])
	}
	run := doc["runs"].([]any)[0].(map[string]any)
	driver := run["tool"].(map[string]any)["driver"].(map[string]any)
	if driver["version"] != Version {
		t.Fatalf("driver version = %v, want %v", driver["version"], Version)
	}
	results := run["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("SARIF results count = %d, want 3", len(results))
	}
	// Result 0: zero_line violation -> region omitted
	res0 := results[0].(map[string]any)
	loc0 := res0["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)
	if _, hasRegion := loc0["region"]; hasRegion {
		t.Fatalf("zero_line violation emitted region: %v", loc0["region"])
	}
	// Result 1: valid_line violation -> region present
	res1 := results[1].(map[string]any)
	loc1 := res1["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)
	if _, hasRegion := loc1["region"]; !hasRegion {
		t.Fatalf("valid_line violation missing region")
	}
	// Result 2: error -> region omitted
	res2 := results[2].(map[string]any)
	loc2 := res2["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)
	if _, hasRegion := loc2["region"]; hasRegion {
		t.Fatalf("SARIF error emitted region for whole-file error: %v", loc2["region"])
	}
}

func TestGitHubEscapesWorkflowCommands(t *testing.T) {
	r := &rule.Base{RuleName: "Stub"}
	var buf bytes.Buffer
	err := (GitHubRenderer{}).Render(&buf, &Report{Violations: []*rule.Violation{{
		File: "a,b.go", BeginLine: 1, Description: "first\n::error::injected", Rule: r,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "\n::error::injected") {
		t.Fatalf("unescaped newline injected a second workflow command:\n%s", out)
	}
}

func TestFormatsSurfaceParseErrors(t *testing.T) {
	rep := &Report{Errors: []ProcessingError{{File: "bad.go", Message: "parse failed"}}}
	for _, format := range []string{"gitlab", "checkstyle", "sarif", "html"} {
		renderer, ok := For(format)
		if !ok {
			t.Fatalf("missing renderer %s", format)
		}
		var buf bytes.Buffer
		if err := renderer.Render(&buf, rep); err != nil {
			t.Errorf("%s: %v", format, err)
			continue
		}
		if !strings.Contains(buf.String(), "parse failed") && !strings.Contains(buf.String(), "bad.go") {
			t.Errorf("%s dropped Report.Errors:\n%s", format, buf.String())
		}
	}
}
