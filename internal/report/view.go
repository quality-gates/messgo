package report

import (
	"iter"

	"github.com/quality-gates/messgo/internal/rule"
)

// View retains report order and provides the file projections used by renderers.
type View struct {
	*Report
}

// NewView creates a view over the given report.
func NewView(r *Report) *View {
	return &View{Report: r}
}

type fileRun struct {
	file       string
	wrapped    bool
	violations iter.Seq[*rule.Violation]
}

func (v *View) fileRuns() iter.Seq[fileRun] {
	return func(yield func(fileRun) bool) {
		violations := v.Violations
		i := 0
		var pending *rule.Violation
		for pending != nil || i < len(violations) {
			first := pending
			if first == nil {
				first = violations[i]
				i++
			}
			pending = nil
			file := first.File
			wrapped := i != 1 || file != ""
			run := fileRun{file: file, wrapped: wrapped}
			run.violations = runViolations(violations, &i, &pending, file, first)
			if !yield(run) {
				return
			}
		}
	}
}

func runViolations(violations []*rule.Violation, i *int, pending **rule.Violation, file string, first *rule.Violation) iter.Seq[*rule.Violation] {
	return func(yield func(*rule.Violation) bool) {
		if !yield(first) {
			return
		}
		for *i < len(violations) {
			item := violations[*i]
			*i++
			if item.File != file {
				*pending = item
				return
			}
			if !yield(item) {
				return
			}
		}
	}
}

func (v *View) indexedViolations() iter.Seq2[int, *rule.Violation] {
	return func(yield func(int, *rule.Violation) bool) {
		indices := map[string]int{}
		for _, violation := range v.Violations {
			index, ok := indices[violation.File]
			if !ok {
				index = len(indices)
				indices[violation.File] = index
			}
			if !yield(index, violation) {
				return
			}
		}
	}
}

var gitlabSeverities = [...]string{"info", "blocker", "critical", "major", "minor"}

func (v *View) gitlabSeverity(priority int) string {
	if priority > 0 && priority < len(gitlabSeverities) {
		return gitlabSeverities[priority]
	}
	return gitlabSeverities[0]
}

func (v *View) checkstyleSeverity(priority int) string {
	if priority <= 2 {
		return "error"
	}
	if priority == 3 {
		return "warning"
	}
	return "info"
}

func (v *View) sarifLevel(priority int) string {
	if priority <= 2 {
		return "error"
	}
	return "warning"
}
