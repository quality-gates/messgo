package runner

import (
	"os"
	"path/filepath"
	"strings"
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

func TestShouldIncludeFileAppliesAllFileFilters(t *testing.T) {
	cases := []struct {
		name string
		path string
		opts Options
		want bool
	}{
		{name: "matching suffix", path: "source.go", opts: Options{Suffixes: []string{".go"}}, want: true},
		{name: "wrong suffix", path: "source.txt", opts: Options{Suffixes: []string{".go"}}, want: false},
		{name: "test file included", path: "source_test.go", opts: Options{Suffixes: []string{".go"}}, want: true},
		{name: "test file ignored", path: "source_test.go", opts: Options{Suffixes: []string{".go"}, IgnoreTests: true}, want: false},
		{name: "excluded path", path: "skip/source.go", opts: Options{Suffixes: []string{".go"}, Exclude: []string{"", "skip"}}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldIncludeFile(tc.path, tc.opts); got != tc.want {
				t.Fatalf("shouldIncludeFile(%q, %+v) = %v, want %v", tc.path, tc.opts, got, tc.want)
			}
		})
	}
}

func TestRunFiltersDiscoveredFiles(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"included.go": "package fixture\nfunc broken( {\n",
		"ignored.txt": "not Go source\n",
		"excluded.go": "package fixture\nfunc broken( {\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	rep, err := Run(Options{Paths: []string{dir}, Exclude: []string{"excluded.go"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Errors) != 1 || filepath.Base(rep.Errors[0].File) != "included.go" {
		t.Fatalf("Run errors = %+v, want only included.go parse error", rep.Errors)
	}
}

func TestDiscoverAcceptsGoWildcard(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := discover(Options{Paths: []string{filepath.Join(dir, "...")}, Suffixes: []string{".go"}})
	if err != nil {
		t.Fatalf("discover ... failed: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("advertised ... path found no files")
	}
}

func TestExternalTestPackageDoesNotMutateProductionGlobal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package p\nvar shared = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package p_test\nvar shared = 1\nfunc mutate() { shared++ }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sets, err := (&ruleset.Loader{}).Load("design")
	if err != nil {
		t.Fatal(err)
	}
	ruleset.FilterRules(sets, []string{"GlobalVariable"}, nil)
	rep, err := Run(Options{Paths: []string{dir}, RuleSets: sets})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range rep.Violations {
		if v.Rule.Name() == "GlobalVariable" && strings.HasSuffix(v.File, "main.go") {
			t.Fatalf("production shared reported mutable because p_test.shared is mutated")
		}
	}
}

func TestMixedRelAbsPathsSharePackage(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	if err := os.WriteFile(a, []byte("package p\nvar shared = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("package p\nfunc mutate() { shared++ }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	absB, err := filepath.Abs(b)
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	sets, err := (&ruleset.Loader{}).Load("design")
	if err != nil {
		t.Fatal(err)
	}
	ruleset.FilterRules(sets, []string{"GlobalVariable"}, nil)
	rep, err := Run(Options{Paths: []string{"a.go", absB}, RuleSets: sets})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range rep.Violations {
		if v.Rule.Name() == "GlobalVariable" {
			found = true
		}
	}
	if !found {
		t.Fatal("mixed relative/absolute paths hid a cross-file global mutation")
	}
}
