// Package report renders rule violations in the output formats PHPMD
// supports: text, xml, json, ansi, github, gitlab, checkstyle, sarif, html.
package report

import (
	"io"

	"github.com/quality-gates/messgo/internal/rule"
)

// Version is the messgo version reported in machine-readable output.
const Version = "0.1.4"

// ProcessingError is a file that could not be parsed.
type ProcessingError struct {
	File    string
	Message string
}

// Report is the full set of results handed to a renderer.
type Report struct {
	Violations []*rule.Violation
	Errors     []ProcessingError
}

// Renderer writes a Report to a writer.
type Renderer interface {
	Render(w io.Writer, r *Report) error
}

var renderers = map[string]func() Renderer{
	"text":       func() Renderer { return &TextRenderer{} },
	"ansi":       func() Renderer { return &TextRenderer{Colored: true} },
	"xml":        func() Renderer { return &XMLRenderer{} },
	"json":       func() Renderer { return &JSONRenderer{} },
	"github":     func() Renderer { return &GitHubRenderer{} },
	"gitlab":     func() Renderer { return &GitLabRenderer{} },
	"checkstyle": func() Renderer { return &CheckStyleRenderer{} },
	"sarif":      func() Renderer { return &SARIFRenderer{} },
	"html":       func() Renderer { return &HTMLRenderer{} },
}

// For returns the renderer for a PHPMD format identifier.
func For(format string) (Renderer, bool) {
	if ctor, ok := renderers[format]; ok {
		return ctor(), true
	}
	return nil, false
}

// Formats lists the supported renderer identifiers.
func Formats() []string {
	return []string{"text", "xml", "json", "html", "ansi", "github", "gitlab", "checkstyle", "sarif"}
}
