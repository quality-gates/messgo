package report

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/quality-gates/messgo/internal/rule"
)

type callbackWriter struct {
	bytes.Buffer
	callback func(string)
	err      error
}

func (w *callbackWriter) Write(p []byte) (int, error) {
	if w.callback != nil {
		w.callback(string(p))
	}
	if w.err != nil {
		return 0, w.err
	}
	return w.Buffer.Write(p)
}

func TestStreamRenderersReadSlicesAtRangeStart(t *testing.T) {
	for _, format := range []string{"xml", "checkstyle", "html"} {
		for _, change := range []string{"replace", "append"} {
			t.Run(format+"/"+change, func(t *testing.T) {
				rep := rendererTestReport()
				original := rep.Violations
				rep.Violations = original[:1:1]
				w := &callbackWriter{}
				calls := 0
				w.callback = func(s string) {
					calls++
					if calls == 1 {
						if change == "replace" {
							rep.Violations = []*rule.Violation{original[1]}
						} else {
							rep.Violations = append(rep.Violations, original[1])
						}
					}
					if strings.Contains(s, "first violation") || strings.Contains(s, "second violation") {
						rep.Violations = append(rep.Violations, original[0])
						rep.Errors = []ProcessingError{{File: "new.go", Message: "new error"}}
					}
				}
				renderer, _ := For(format)
				if err := renderer.Render(w, rep); err != nil {
					t.Fatal(err)
				}
				got := w.String()
				wantFirst := 0
				if change == "append" {
					wantFirst = 1
				}
				if strings.Count(got, "second violation") != 1 || strings.Count(got, "first violation") != wantFirst || !strings.Contains(got, "new error") || strings.Contains(got, "parse failed") {
					t.Fatalf("unexpected output: %s", got)
				}
			})
		}
	}
}

func TestTextAndGitHubReadErrorsAfterWritingViolations(t *testing.T) {
	for _, format := range []string{"text", "ansi", "github"} {
		t.Run(format, func(t *testing.T) {
			rep := rendererTestReport()
			w := &callbackWriter{callback: func(string) { rep.Errors = []ProcessingError{{File: "new.go", Message: "new error"}} }}
			renderer, _ := For(format)
			if err := renderer.Render(w, rep); err != nil {
				t.Fatal(err)
			}
			if got := w.String(); !strings.Contains(got, "new error") || strings.Contains(got, "parse failed") {
				t.Fatalf("unexpected output: %s", got)
			}
		})
	}
}

func TestStreamRenderersWriteHeadersBeforeNilReportPanic(t *testing.T) {
	for _, format := range []string{"xml", "checkstyle", "html"} {
		t.Run(format, func(t *testing.T) {
			var out bytes.Buffer
			renderer, _ := For(format)
			func() {
				defer func() {
					if recover() == nil {
						t.Error("expected panic")
					}
				}()
				_ = renderer.Render(&out, nil)
			}()
			got := out.String()
			switch format {
			case "xml":
				if !strings.HasPrefix(got, "<?xml version=\"1.0\" encoding=\"UTF-8\" ?>\n<pmd version=\"") || !strings.HasSuffix(got, "\">\n") {
					t.Fatalf("prefix = %q", got)
				}
			case "checkstyle":
				want := fmt.Sprintf("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<checkstyle version=\"%s\">\n", Version)
				if got != want {
					t.Fatalf("prefix = %q, want %q", got, want)
				}
			case "html":
				want := "<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"><title>messgo report</title></head><body>\n<h1>messgo report</h1>\n"
				if got != want {
					t.Fatalf("prefix = %q, want %q", got, want)
				}
			}
		})
	}
}

func TestStreamRenderersContinueAfterHeaderWriteError(t *testing.T) {
	for _, format := range []string{"xml", "checkstyle", "html"} {
		t.Run(format, func(t *testing.T) {
			rep := rendererTestReport()
			want := errors.New("write failed")
			w := &callbackWriter{err: want, callback: func(string) { rep.Violations = []*rule.Violation{nil} }}
			renderer, _ := For(format)
			defer func() {
				if recover() == nil {
					t.Error("expected panic while processing changed violations after write error")
				}
			}()
			_ = renderer.Render(w, rep)
		})
	}
}

func TestStreamRenderersKeepSelectedViolationAcrossFileClose(t *testing.T) {
	for _, format := range []string{"xml", "checkstyle", "html"} {
		for _, change := range []string{"replace pointer", "mutate file"} {
			t.Run(format+"/"+change, func(t *testing.T) {
				rep := rendererTestReport()
				rep.Errors = nil
				pending := rep.Violations[1]
				closeTag := "  </file>\n"
				fileTag := `  <file name="b.go">`
				if format == "html" {
					closeTag = "</table>\n"
					fileTag = "<h2>b.go</h2>"
				}
				changed := false
				w := &callbackWriter{callback: func(s string) {
					if s != closeTag || changed {
						return
					}
					changed = true
					if change == "replace pointer" {
						replacement := *pending
						replacement.File = "c.go"
						replacement.Description = "replacement violation"
						rep.Violations[1] = &replacement
					} else {
						pending.File = "c.go"
					}
				}}
				if change == "mutate file" {
					fileTag = strings.Replace(fileTag, "b.go", "c.go", 1)
				}
				renderer, _ := For(format)
				if err := renderer.Render(w, rep); err != nil {
					t.Fatal(err)
				}
				got := w.String()
				if !changed || !strings.Contains(got, fileTag) || !strings.Contains(got, "second violation") || strings.Contains(got, "replacement violation") {
					t.Fatalf("unexpected output: %s", got)
				}
			})
		}
	}
}

func TestStreamRenderersFileRunBoundaries(t *testing.T) {
	for _, format := range []string{"xml", "checkstyle", "html"} {
		t.Run(format, func(t *testing.T) {
			rep := rendererTestReport()
			base := *rep.Violations[0]
			rep.Violations = nil
			rep.Errors = nil
			for i, file := range []string{"", "", "a.go", "a.go", "", "b.go"} {
				v := base
				v.File = file
				v.Description = fmt.Sprintf("item-%d", i)
				rep.Violations = append(rep.Violations, &v)
			}
			var out bytes.Buffer
			renderer, _ := For(format)
			if err := renderer.Render(&out, rep); err != nil {
				t.Fatal(err)
			}
			got := out.String()
			tags := []string{`<file name="a.go">`, `<file name="">`, `<file name="b.go">`}
			if format == "html" {
				tags = []string{"<h2>a.go</h2>", "<h2></h2>", "<h2>b.go</h2>"}
			}
			parts := []string{"item-0", "item-1", tags[0], "item-2", "item-3", tags[1], "item-4", tags[2], "item-5"}
			pos := 0
			for _, part := range parts {
				i := strings.Index(got[pos:], part)
				if i < 0 || strings.Count(got, part) != 1 {
					t.Fatalf("missing, repeated, or out-of-order %q in output: %s", part, got)
				}
				pos += i + len(part)
			}
		})
	}
}
