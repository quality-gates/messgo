package report

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/quality-gates/messgo/internal/rule"
)

func characterizationReport() *Report {
	first := &rule.Base{RuleName: "Repeat<&", RuleURL: "https://example.test/rule?a=1&b=2", RuleDesc: "  first <description>  "}
	duplicate := &rule.Base{RuleName: "Repeat<&", RuleURL: "https://other.test/", RuleDesc: "second metadata"}
	other := &rule.Base{RuleName: "Other", RuleURL: "relative", RuleDesc: "other rule"}
	return &Report{
		Violations: []*rule.Violation{
			{Rule: first, File: "a.go", BeginLine: 1, EndLine: 2, Description: "first & <tag> \"'", Priority: 1, RuleSetName: "Set&One", Package: "pkg", Function: "fn"},
			{Rule: other, File: "b,:%.go", BeginLine: 0, Description: "line\nnext %\r", Priority: 2, RuleSetName: "Set"},
			{Rule: duplicate, File: "a.go", BeginLine: 3, EndLine: 4, Description: "duplicate", Priority: 3, RuleSetName: "OtherSet", Class: "C", Method: "M"},
			{Rule: first, File: "", BeginLine: 4, EndLine: 4, Description: "empty file", Priority: 4},
			{Rule: other, File: "a.go", BeginLine: 5, EndLine: 5, Description: "five", Priority: 5},
			{Rule: other, File: "a.go", BeginLine: 6, EndLine: 6, Description: "zero", Priority: 0},
			{Rule: other, File: "a.go", BeginLine: 7, EndLine: 7, Description: "negative", Priority: -1},
			{Rule: other, File: "a.go", BeginLine: 8, EndLine: 8, Description: "high", Priority: 6},
		},
		Errors: []ProcessingError{{File: "bad<&\".go", Message: "parse & <failed>\nnext"}, {File: "", Message: "empty error"}},
	}
}

var timestampPattern = regexp.MustCompile(`(timestamp="|"timestamp": ")([^"\n]+)(")`)

func normalizeTimestamp(t *testing.T, format, output string) string {
	t.Helper()
	if format != "xml" && format != "json" {
		return output
	}
	matches := timestampPattern.FindAllStringSubmatchIndex(output, -1)
	if len(matches) != 1 {
		t.Fatalf("%s: got %d timestamps, want 1", format, len(matches))
	}
	m := matches[0]
	stamp := output[m[4]:m[5]]
	if _, err := time.Parse(time.RFC3339, stamp); err != nil {
		t.Fatalf("%s: timestamp %q is not RFC3339: %v", format, stamp, err)
	}
	return output[:m[4]] + "TIMESTAMP" + output[m[5]:]
}

const ansiCharacterization = "a.go:1     \u001b[33mRepeat<&\u001b[0m  \u001b[31mfirst & <tag> \"'\u001b[0m\n" +
	"b,:%.go:0  \u001b[33mOther\u001b[0m     \u001b[31mline\n" +
	"next %\r" +
	"\u001b[0m\n" +
	"a.go:3     \u001b[33mRepeat<&\u001b[0m  \u001b[31mduplicate\u001b[0m\n" +
	":4         \u001b[33mRepeat<&\u001b[0m  \u001b[31mempty file\u001b[0m\n" +
	"a.go:5     \u001b[33mOther\u001b[0m     \u001b[31mfive\u001b[0m\n" +
	"a.go:6     \u001b[33mOther\u001b[0m     \u001b[31mzero\u001b[0m\n" +
	"a.go:7     \u001b[33mOther\u001b[0m     \u001b[31mnegative\u001b[0m\n" +
	"a.go:8     \u001b[33mOther\u001b[0m     \u001b[31mhigh\u001b[0m\n" +
	"bad<&\".go\t-\tparse & <failed>\n" +
	"next\n" +
	"\t-\tempty error\n"

func TestRendererCharacterization(t *testing.T) {
	oldVersion := Version
	Version = "test-version"
	t.Cleanup(func() { Version = oldVersion })
	for _, format := range []string{"text", "xml", "json", "html", "ansi", "github", "gitlab", "checkstyle", "sarif"} {
		t.Run(format, func(t *testing.T) {
			got := normalizeTimestamp(t, format, renderReport(t, format, characterizationReport()))
			want := ansiCharacterization
			if format != "ansi" {
				golden, err := os.ReadFile(filepath.Join("testdata", "characterization", format+".golden"))
				if err != nil {
					t.Fatal(err)
				}
				want = strings.ReplaceAll(string(golden), "<CR>", "\r")
			}
			if got != want {
				t.Fatalf("output differs for %s\ngot:\n%s\nwant:\n%s", format, got, want)
			}
		})
	}
}

func TestRendererInitialEmptyFilename(t *testing.T) {
	r := &rule.Base{RuleName: "R"}
	rep := &Report{Violations: []*rule.Violation{{Rule: r, File: "", BeginLine: 0, Description: "empty"}, {Rule: r, File: "a", BeginLine: 1, Description: "named"}}}
	oldVersion := Version
	Version = "test-version"
	t.Cleanup(func() { Version = oldVersion })
	for _, format := range []string{"xml", "checkstyle", "html"} {
		t.Run(format, func(t *testing.T) {
			got := normalizeTimestamp(t, format, renderReport(t, format, rep))
			want, err := os.ReadFile(filepath.Join("testdata", "characterization", "initial-empty-"+format+".golden"))
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("output differs\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

type prefixWriter struct {
	bytes.Buffer
	limit int
	err   error
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.Len()
	if remaining < len(p) {
		if remaining < 0 {
			remaining = 0
		}
		w.Buffer.Write(p[:remaining])
		return remaining, w.err
	}
	return w.Buffer.Write(p)
}

func TestRendererWriterFailures(t *testing.T) {
	failure := errors.New("write failure")
	for format, prefix := range map[string]string{"text": "a.go:", "xml": "<?xml", "json": "{\n   ", "html": "<!DOC", "ansi": "a.go:", "github": "::war", "gitlab": "[\n   ", "checkstyle": "<?xml", "sarif": "{\n  \""} {
		for _, tc := range []struct {
			name            string
			writerErr, want error
		}{{"error", failure, failure}, {"short", nil, io.ErrShortWrite}} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				renderer, ok := For(format)
				if !ok {
					t.Fatal("missing renderer")
				}
				w := &prefixWriter{limit: 5, err: tc.writerErr}
				err := renderer.Render(w, characterizationReport())
				if !errors.Is(err, tc.want) || w.String() != prefix {
					t.Fatalf("Render = %v, bytes = %q; want %v and %q", err, w.String(), tc.want, prefix)
				}
			})
		}
	}
}
