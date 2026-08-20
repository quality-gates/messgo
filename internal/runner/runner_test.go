package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/quality-gates/messgo/internal/ruleset"
)

func TestRunIgnoreTestsSkipsExplicitTestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture_test.go")
	src := []byte(`package fixture

func tooMany(a, b, c, d, e, f, g, h, i, j, k int) {}
`)
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	sets, err := (&ruleset.Loader{}).Load("codesize")
	if err != nil {
		t.Fatalf("load ruleset: %v", err)
	}

	includeTests, err := Run(Options{Paths: []string{path}, RuleSets: sets})
	if err != nil {
		t.Fatalf("Run without IgnoreTests: %v", err)
	}
	if len(includeTests.Violations) == 0 {
		t.Fatal("expected the explicit test file to be analyzed when IgnoreTests is false")
	}

	skipTests, err := Run(Options{Paths: []string{path}, RuleSets: sets, IgnoreTests: true})
	if err != nil {
		t.Fatalf("Run with IgnoreTests: %v", err)
	}
	if len(skipTests.Violations) != 0 {
		t.Fatalf("IgnoreTests analyzed explicit test file: got %d violations", len(skipTests.Violations))
	}
}
