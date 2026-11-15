package explicitness

import (
	"go/ast"
	"testing"

	"github.com/quality-gates/messgo/internal/model"
)

func TestActiveSummaryReturnsProvisionalResult(t *testing.T) {
	for _, phase := range []summaryPhase{summaryEvaluating, summaryReevaluating} {
		t.Run(map[summaryPhase]string{summaryEvaluating: "initial evaluation", summaryReevaluating: "reevaluation"}[phase], func(t *testing.T) {
			body := &ast.BlockStmt{}
			fn := &model.Function{Body: body}
			entry := state{}
			summary := &storageSummary{fn: fn, entry: entry, phase: phase, callers: map[*storageSummary]bool{}, callees: map[*storageSummary]bool{}}
			cache := &storageSummaryCache{entries: map[*ast.BlockStmt][]*storageSummary{body: {summary}}, current: summary}
			result, recursive := cache.summarize(fn, entry, false)
			if !recursive || result == summary || !result.result.ambient {
				t.Fatal("recursive call must receive a provisional ambient result while the summary is active")
			}
			if !summary.callers[summary] || !summary.callees[summary] {
				t.Fatal("recursive call must register the summary dependency")
			}
		})
	}
}
