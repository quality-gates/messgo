// Package report renders rule violations in the output formats PHPMD
// supports: text, xml, json, ansi, github, gitlab, checkstyle, sarif, html.
package report

import (
	"io"

	"github.com/quality-gates/messgo/internal/rule"
)

// Version is the messgo version reported in machine-readable output.
const Version = "1.0.0"

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

// For returns the renderer for a PHPMD format identifier.
func For(format string) (Renderer, bool) {
	switch format {
	case "text":
		return &TextRenderer{}, true
	case "ansi":
		return &TextRenderer{Colored: true}, true
	case "xml":
		return &XMLRenderer{}, true
	case "json":
		return &JSONRenderer{}, true
	case "github":
		return &GitHubRenderer{}, true
	case "gitlab":
		return &GitLabRenderer{}, true
	case "checkstyle":
		return &CheckStyleRenderer{}, true
	case "sarif":
		return &SARIFRenderer{}, true
	case "html":
		return &HTMLRenderer{}, true
	}
	return nil, false
}

// Formats lists the supported renderer identifiers.
func Formats() []string {
	return []string{"text", "xml", "json", "html", "ansi", "github", "gitlab", "checkstyle", "sarif"}
}
