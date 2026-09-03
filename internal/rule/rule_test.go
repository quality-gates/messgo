package rule

import (
	"testing"

	"github.com/quality-gates/messgo/internal/model"
)

func TestRenderMessage(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		args []any
		want string
	}{
		{"no placeholders", "plain message", nil, "plain message"},
		{"single string arg", "found {0}", []any{"foo"}, "found foo"},
		{"reordered args", "{1} before {0}", []any{"a", "b"}, "b before a"},
		{"int arg", "complexity {0}", []any{7}, "complexity 7"},
		{"int64 arg", "count {0}", []any{int64(42)}, "count 42"},
		{"integral float has no decimal", "value {0}", []any{float64(3)}, "value 3"},
		{"fractional float kept", "value {0}", []any{1.5}, "value 1.5"},
		{"out-of-range index left intact", "only {0} and {3}", []any{"x"}, "only x and {3}"},
		{"unsupported arg renders empty", "got {0}", []any{[]int{1}}, "got "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RenderMessage(tt.tmpl, tt.args); got != tt.want {
				t.Errorf("RenderMessage(%q, %v) = %q, want %q", tt.tmpl, tt.args, got, tt.want)
			}
		})
	}
}

func TestPropertiesInt(t *testing.T) {
	p := Properties{"n": "5", "bad": "notnum"}
	if got := p.Int("n", 1); got != 5 {
		t.Errorf("Int present = %d, want 5", got)
	}
	if got := p.Int("missing", 9); got != 9 {
		t.Errorf("Int missing = %d, want 9 (default)", got)
	}
	if got := p.Int("bad", 9); got != 9 {
		t.Errorf("Int unparsable = %d, want 9 (default)", got)
	}
}

func TestPropertiesFloat(t *testing.T) {
	p := Properties{"f": "2.5", "bad": "x"}
	if got := p.Float("f", 1); got != 2.5 {
		t.Errorf("Float present = %v, want 2.5", got)
	}
	if got := p.Float("missing", 1.25); got != 1.25 {
		t.Errorf("Float missing = %v, want 1.25 (default)", got)
	}
	if got := p.Float("bad", 1.25); got != 1.25 {
		t.Errorf("Float unparsable = %v, want 1.25 (default)", got)
	}
}

func TestPropertiesBool(t *testing.T) {
	p := Properties{
		"t1": "true", "t2": "1", "t3": "yes", "t4": "on",
		"f1": "false", "f2": "0", "f3": "no", "f4": "off",
		"weird": "maybe",
	}
	for _, k := range []string{"t1", "t2", "t3", "t4"} {
		if !p.Bool(k, false) {
			t.Errorf("Bool(%q) = false, want true", k)
		}
	}
	for _, k := range []string{"f1", "f2", "f3", "f4"} {
		if p.Bool(k, true) {
			t.Errorf("Bool(%q) = true, want false", k)
		}
	}
	if !p.Bool("weird", true) {
		t.Error("Bool unrecognized value should fall back to default true")
	}
	if p.Bool("missing", false) {
		t.Error("Bool missing should fall back to default false")
	}
}

func TestPropertiesString(t *testing.T) {
	p := Properties{"s": "value"}
	if got := p.String("s", "def"); got != "value" {
		t.Errorf("String present = %q, want %q", got, "value")
	}
	if got := p.String("missing", "def"); got != "def" {
		t.Errorf("String missing = %q, want %q", got, "def")
	}
}

func TestCompileRegex(t *testing.T) {
	if CompileRegex("") != nil {
		t.Error("empty pattern should compile to nil")
	}
	// Delimited PHPMD form with a case-insensitive flag.
	re := CompileRegex("(^(set|get|is|has))i")
	if re == nil {
		t.Fatal("valid delimited pattern compiled to nil")
	}
	if !re.MatchString("GETTER") {
		t.Error("expected case-insensitive match for GETTER")
	}
	if re.MatchString("delete") {
		t.Error("did not expect a match for delete")
	}
	// Plain pattern without delimiters still compiles.
	if CompileRegex("^foo$") == nil {
		t.Error("plain pattern should compile")
	}
	slash := CompileRegex("/^get/i")
	if slash == nil || !slash.MatchString("GETValue") {
		t.Error("PHPMD slash-delimited regex /^get/i should match GETValue")
	}
	// Invalid pattern returns nil rather than panicking.
	if CompileRegex("(") != nil {
		t.Error("invalid pattern should compile to nil")
	}
}

func TestSortViolations(t *testing.T) {
	vs := []*Violation{
		{File: "b.go", BeginLine: 1},
		{File: "a.go", BeginLine: 10},
		{File: "a.go", BeginLine: 2},
	}
	SortViolations(vs)
	got := []struct {
		file string
		line int
	}{}
	for _, v := range vs {
		got = append(got, struct {
			file string
			line int
		}{v.File, v.BeginLine})
	}
	want := []struct {
		file string
		line int
	}{{"a.go", 2}, {"a.go", 10}, {"b.go", 1}}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestReportFuncUsesTargetFile(t *testing.T) {
	var violations []*Violation
	r := &Base{RuleName: "TestRule"}
	c := &Context{
		File:       &model.File{Path: "main.go", Package: "main"},
		rule:       r,
		violations: &violations,
	}

	// 1. Function has explicit file with path
	fnWithFile := &model.Function{
		Name:    "M",
		Line:    10,
		EndLine: 12,
		File:    &model.File{Path: "other.go", Package: "otherpkg"},
	}
	c.ReportFunc(fnWithFile)
	if len(violations) != 1 || violations[0].File != "other.go" || violations[0].Package != "otherpkg" {
		t.Fatalf("expected other.go/otherpkg, got %v", violations[0])
	}

	// 2. Function has file with empty path (should fallback to context file)
	fnEmptyPath := &model.Function{
		Name:    "M2",
		Line:    20,
		EndLine: 22,
		File:    &model.File{Path: "", Package: "fallback"},
	}
	c.ReportFunc(fnEmptyPath)
	if len(violations) != 2 || violations[1].File != "main.go" || violations[1].Package != "main" {
		t.Fatalf("expected fallback to main.go/main, got %v", violations[1])
	}

	// 3. Function has nil File (should fallback to context file)
	fnNilFile := &model.Function{
		Name:    "M3",
		Line:    30,
		EndLine: 32,
		File:    nil,
	}
	c.ReportFunc(fnNilFile)
	if len(violations) != 3 || violations[2].File != "main.go" || violations[2].Package != "main" {
		t.Fatalf("expected fallback to main.go/main, got %v", violations[2])
	}
}
