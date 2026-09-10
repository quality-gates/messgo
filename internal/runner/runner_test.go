package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quality-gates/messgo/internal/model"
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
		{name: "dot-prefix exclude against cleaned path", path: "proj/gen/g.go", opts: Options{Suffixes: []string{".go"}, Exclude: []string{"./proj/gen"}}, want: false},
		{name: "cleaned exclude against dotted path", path: "./proj/gen/g.go", opts: Options{Suffixes: []string{".go"}, Exclude: []string{"proj/gen"}}, want: false},
		{name: "dot-prefix exclude misses sibling", path: "proj/ok/o.go", opts: Options{Suffixes: []string{".go"}, Exclude: []string{"./proj/gen"}}, want: true},
		{name: "empty exclude does not match", path: "source.go", opts: Options{Suffixes: []string{".go"}, Exclude: []string{""}}, want: true},
		{name: "unclean path still matches cleaned exclude", path: "proj/./gen/g.go", opts: Options{Suffixes: []string{".go"}, Exclude: []string{"proj/gen"}}, want: false},
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

func TestDirectoryDiscoverySkipsHiddenAndUnderscoreDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".worktrees", "feature"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "_tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".worktrees", "feature", "code.go"), []byte("package feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_tools", "code.go"), []byte("package tools\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "code.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := discover(Options{Paths: []string{dir}, Suffixes: []string{".go"}})
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "code.go" || !strings.Contains(files[0], "pkg") {
		t.Fatalf("discover returned unexpected files: %v, want only pkg/code.go", files)
	}

	// Explicit path to hidden directory root should still be scanned
	explicitFiles, err := discover(Options{Paths: []string{filepath.Join(dir, ".worktrees")}, Suffixes: []string{".go"}})
	if err != nil {
		t.Fatalf("explicit discover failed: %v", err)
	}
	if len(explicitFiles) != 1 {
		t.Fatalf("explicit discover of hidden directory failed: %v", explicitFiles)
	}
}

func TestDirectoryDiscoverySkipsHiddenAndUnderscoreFiles(t *testing.T) {
	dir := t.TempDir()
	src := "package fixture\ntype hidden struct {\n\tunused int\n}\n"
	for _, name := range []string{"visible.go", ".ignored.go", "_ignored.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	sets, err := (&ruleset.Loader{}).Load("unusedcode")
	if err != nil {
		t.Fatalf("load ruleset: %v", err)
	}
	ruleset.FilterRules(sets, []string{"UnusedPrivateField"}, nil)

	recursive, err := Run(Options{Paths: []string{dir}, RuleSets: sets})
	if err != nil {
		t.Fatalf("recursive Run: %v", err)
	}
	if len(recursive.Violations) != 1 || filepath.Base(recursive.Violations[0].File) != "visible.go" {
		t.Errorf("recursive Run violations = %+v, want only visible.go", recursive.Violations)
	}

	for _, name := range []string{".ignored.go", "_ignored.go"} {
		explicit, err := Run(Options{Paths: []string{filepath.Join(dir, name)}, RuleSets: sets})
		if err != nil {
			t.Fatalf("explicit Run for %s: %v", name, err)
		}
		if len(explicit.Violations) != 1 || filepath.Base(explicit.Violations[0].File) != name {
			t.Fatalf("explicit Run for %s violations = %+v, want one finding", name, explicit.Violations)
		}
	}
}

func TestShouldSkipDir(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{".", false},
		{"..", false},
		{".git", true},
		{".worktrees", true},
		{"_tools", true},
		{"vendor", true},
		{"node_modules", true},
		{"pkg", false},
	}
	for _, tt := range tests {
		if got := shouldSkipDir(tt.name); got != tt.want {
			t.Errorf("shouldSkipDir(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestAttachPackageMethods(t *testing.T) {
	f1, err := model.ParseSource("a.go", []byte("package p\ntype S struct{}\n"))
	if err != nil {
		t.Fatal(err)
	}
	f2, err := model.ParseSource("b.go", []byte("package p\nfunc (s *S) M() {}\nfunc Free() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	c := f1.Classes[0]
	annotatePackages([]*model.File{f1, f2})
	var fnMethod, fnFree *model.Function
	for _, fn := range f2.AllFuncs {
		if fn.Name == "M" {
			fnMethod = fn
		} else if fn.Name == "Free" {
			fnFree = fn
		}
	}
	if fnMethod == nil || fnMethod.Class != c {
		t.Fatalf("fnMethod.Class = %v, want %v", fnMethod.Class, c)
	}
	if fnFree == nil || fnFree.Class != nil {
		t.Fatalf("fnFree.Class = %v, want nil", fnFree.Class)
	}
	if len(c.Methods) != 1 || c.Methods[0] != fnMethod {
		t.Fatalf("c.Methods = %v, want [fnMethod]", c.Methods)
	}
}

func TestCrossFileMethodAttachmentForClassRules(t *testing.T) {
	dir := t.TempDir()
	structCode := "package service\ntype Service struct{}\n"
	var methodCode strings.Builder
	methodCode.WriteString("package service\n")
	for i := 1; i <= 26; i++ {
		methodCode.WriteString(strings.ReplaceAll("func (s *Service) M{X}() {}\n", "{X}", string(rune('A'+(i%26)))))
	}
	// Write with unique names
	var methodCodeUnique strings.Builder
	methodCodeUnique.WriteString("package service\n")
	for i := 1; i <= 26; i++ {
		methodCodeUnique.WriteString(strings.Repeat(" ", 0))
		methodCodeUnique.WriteString("func (s *Service) Method" + string(rune('A'+i-1)) + "() {}\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "service.go"), []byte(structCode), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "methods.go"), []byte(methodCodeUnique.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	sets, err := (&ruleset.Loader{}).Load("codesize")
	if err != nil {
		t.Fatal(err)
	}
	ruleset.FilterRules(sets, []string{"TooManyMethods"}, nil)
	rep, err := Run(Options{Paths: []string{dir}, RuleSets: sets})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Violations) != 1 {
		t.Fatalf("TooManyMethods across files reported %d violations, want 1", len(rep.Violations))
	}
	if rep.Violations[0].Class != "Service" {
		t.Fatalf("TooManyMethods violation on class %s, want Service", rep.Violations[0].Class)
	}
}

func TestCrossFileMemberSelectedPreventsUnusedWarning(t *testing.T) {
	dir := t.TempDir()
	modelCode := `package account
type Account struct {
	balance int
	unusedField int
}
func (a *Account) calculateInterest() {}
func (a *Account) unusedMethod() {}
`
	serviceCode := `package account
func Audit(a *Account) {
	println(a.balance)
	a.calculateInterest()
}
`
	if err := os.WriteFile(filepath.Join(dir, "model.go"), []byte(modelCode), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "service.go"), []byte(serviceCode), 0o644); err != nil {
		t.Fatal(err)
	}

	sets, err := (&ruleset.Loader{}).Load("unusedcode")
	if err != nil {
		t.Fatal(err)
	}
	ruleset.FilterRules(sets, []string{"UnusedPrivateField", "UnusedPrivateMethod"}, nil)
	rep, err := Run(Options{Paths: []string{dir}, RuleSets: sets})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Violations) != 2 {
		t.Fatalf("got %d violations, want exactly 2 (unusedField and unusedMethod)", len(rep.Violations))
	}
	for _, v := range rep.Violations {
		if strings.Contains(v.Description, "balance") || strings.Contains(v.Description, "calculateInterest") {
			t.Fatalf("falsely reported cross-file used member: %s", v.Description)
		}
	}
}

func TestCrossFileMemberSelectionScopesUnrelatedTypes(t *testing.T) {
	dir := t.TempDir()
	declarations := `package account

type S struct {
	secret int
}

type T struct {
	secret int
}

func (S) do() {}
func (T) do() {}
`
	uses := `package account

var _ = T{secret: 1}

func use(t T) {
	t.do()
}
`
	if err := os.WriteFile(filepath.Join(dir, "model.go"), []byte(declarations), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "service.go"), []byte(uses), 0o644); err != nil {
		t.Fatal(err)
	}

	sets, err := (&ruleset.Loader{}).Load("unusedcode")
	if err != nil {
		t.Fatal(err)
	}
	ruleset.FilterRules(sets, []string{"UnusedPrivateField", "UnusedPrivateMethod"}, nil)
	rep, err := Run(Options{Paths: []string{dir}, RuleSets: sets})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Violations) != 2 {
		t.Fatalf("got %d violations, want S.secret and S.do: %+v", len(rep.Violations), rep.Violations)
	}
	var fieldFound, methodFound bool
	for _, violation := range rep.Violations {
		switch violation.Rule.Name() {
		case "UnusedPrivateField":
			fieldFound = violation.BeginLine == 4
		case "UnusedPrivateMethod":
			methodFound = violation.Class == "S" && violation.Method == "do"
		}
	}
	if !fieldFound || !methodFound {
		t.Fatalf("violations = %+v, want only S.secret and S.do", rep.Violations)
	}
}

func TestCrossFileUnkeyedLiteralPreventsUnusedFieldWarning(t *testing.T) {
	dir := t.TempDir()
	modelCode := `package account
type key struct {
	typ string
	ptr any
}
`
	serviceCode := `package account
func Seen(seen map[key]bool, typ string, ptr any) bool {
	k := key{typ, ptr}
	if seen[k] {
		return true
	}
	seen[k] = true
	return false
}
`
	if err := os.WriteFile(filepath.Join(dir, "model.go"), []byte(modelCode), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "service.go"), []byte(serviceCode), 0o644); err != nil {
		t.Fatal(err)
	}

	sets, err := (&ruleset.Loader{}).Load("unusedcode")
	if err != nil {
		t.Fatal(err)
	}
	ruleset.FilterRules(sets, []string{"UnusedPrivateField"}, nil)
	rep, err := Run(Options{Paths: []string{dir}, RuleSets: sets})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Violations) != 0 {
		t.Fatalf("violations = %+v, want none: key fields are populated by a sibling-file positional literal", rep.Violations)
	}
}

func TestCrossFileInterfaceSatisfiesMethodFromSiblingFile(t *testing.T) {
	dir := t.TempDir()
	// The interface is declared in the file analyzed after the implementation,
	// so a first-file-only aggregation would miss it.
	modelCode := `package account

type Ledger struct{}

func (l *Ledger) reconcile() {}
`
	ifaceCode := `package account

type Book interface {
	reconcile()
}
`
	if err := os.WriteFile(filepath.Join(dir, "model.go"), []byte(modelCode), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "service.go"), []byte(ifaceCode), 0o644); err != nil {
		t.Fatal(err)
	}

	sets, err := (&ruleset.Loader{}).Load("unusedcode")
	if err != nil {
		t.Fatal(err)
	}
	ruleset.FilterRules(sets, []string{"UnusedPrivateMethod"}, nil)
	rep, err := Run(Options{Paths: []string{dir}, RuleSets: sets})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Violations) != 0 {
		t.Fatalf("violations = %+v, want none: reconcile() satisfies Book declared in a sibling file", rep.Violations)
	}
}

func TestDiscoverExcludeNormalizesDotPrefix(t *testing.T) {
	chdirExcludeFixture(t)
	want := []string{filepath.FromSlash("proj/ok/o.go")}
	cases := []struct {
		name    string
		paths   []string
		exclude []string
	}{
		{name: "walk + dotted exclude", paths: []string{"proj"}, exclude: []string{"./proj/gen"}},
		{name: "walk + plain exclude", paths: []string{"proj"}, exclude: []string{"proj/gen"}},
		{name: "explicit dotted list + dotted exclude", paths: []string{"./proj/gen/g.go", "./proj/ok/o.go"}, exclude: []string{"./proj/gen"}},
		{name: "explicit dotted list + plain exclude", paths: []string{"./proj/gen/g.go", "./proj/ok/o.go"}, exclude: []string{"proj/gen"}},
		{name: "pattern expansion + dotted exclude", paths: []string{"./proj/..."}, exclude: []string{"./proj/gen"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files, err := discover(Options{Paths: tc.paths, Suffixes: []string{".go"}, Exclude: tc.exclude})
			if err != nil {
				t.Fatalf("discover: %v", err)
			}
			if !sameStrings(files, want) {
				t.Fatalf("discover = %v, want %v", files, want)
			}
		})
	}
}

func TestDiscoverReportsCleanedPaths(t *testing.T) {
	chdirExcludeFixture(t)
	want := []string{filepath.FromSlash("proj/gen/g.go"), filepath.FromSlash("proj/ok/o.go")}
	walked, err := discover(Options{Paths: []string{"proj"}, Suffixes: []string{".go"}})
	if err != nil {
		t.Fatalf("walk discover: %v", err)
	}
	explicit, err := discover(Options{Paths: []string{"./proj/gen/g.go", "./proj/ok/o.go"}, Suffixes: []string{".go"}})
	if err != nil {
		t.Fatalf("explicit discover: %v", err)
	}
	if !sameStrings(walked, want) {
		t.Fatalf("walked = %v, want %v", walked, want)
	}
	if !sameStrings(explicit, want) {
		t.Fatalf("explicit = %v, want %v", explicit, want)
	}
}

func TestDiscoverExcludeMatchingNothing(t *testing.T) {
	chdirExcludeFixture(t)
	files, err := discover(Options{Paths: []string{"proj"}, Suffixes: []string{".go"}, Exclude: []string{"./no/such"}})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("discover = %v, want 2 files", files)
	}
}

func chdirExcludeFixture(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, rel := range []string{"proj/gen/g.go", "proj/ok/o.go"} {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package X\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestAttachPackageMethodsResolvesCrossFileTypeAlias(t *testing.T) {
	f1, err := model.ParseSource("a.go", []byte("package p\ntype original struct{}\n"))
	if err != nil {
		t.Fatal(err)
	}
	f2, err := model.ParseSource("b.go", []byte("package p\ntype alias = original\nfunc (alias) m() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	c := f1.Classes[0]
	annotatePackages([]*model.File{f1, f2})
	if len(c.Methods) != 1 || c.Methods[0].Name != "m" {
		t.Fatalf("original.Methods = %v, want [m]", c.Methods)
	}
	if c.Methods[0].Class != c {
		t.Fatalf("m.Class = %v, want original", c.Methods[0].Class)
	}
}
