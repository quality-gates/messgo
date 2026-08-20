package report

import (
	"errors"
	"testing"

	"github.com/quality-gates/messgo/internal/rule"
)

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestTextRendererPropagatesWriterError(t *testing.T) {
	want := errors.New("write failed")
	rep := &Report{Violations: []*rule.Violation{{
		Rule:        &rule.Base{RuleName: "Example"},
		File:        "fixture.go",
		BeginLine:   1,
		Description: "example violation",
	}}}

	err := (&TextRenderer{}).Render(failingWriter{err: want}, rep)
	if !errors.Is(err, want) {
		t.Fatalf("TextRenderer.Render() error = %v, want %v", err, want)
	}
}
