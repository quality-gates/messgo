package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("MESSGO_ADDR_CHILD") == "1" {
		code := Main([]string{os.Getenv("MESSGO_ADDR_PATH"), "json", os.Getenv("MESSGO_ADDR_RULES")}, os.Stdout, os.Stderr)
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestExplicitnessAddressSelectionTerminates(t *testing.T) {
	const reported = `package p

func f() int {
	var s T
	g(&s)
	return s.f
}
`
	cases := []struct {
		name    string
		ruleset string
		source  string
		path    string
		want    []string
		clean   bool
	}{
		{
			name:    "reported fixture",
			ruleset: "explicitness",
			source:  reported,
			want:    []string{"UnknownEffect f: unresolved call"},
		},
		{
			name:    "explicitness-strict",
			ruleset: "explicitness-strict",
			source:  reported,
			want:    []string{"UnknownEffect f: unresolved call"},
		},
		{
			name:    "go and explicitness",
			ruleset: "go,explicitness",
			source:  reported,
			want:    []string{"UnknownEffect f: unresolved call"},
		},
		{
			name:    "go ruleset stays clean",
			ruleset: "go",
			source:  reported,
			clean:   true,
		},
		{
			name:    "address of field then field read",
			ruleset: "explicitness",
			source: `package p

func f() int {
	var s T
	g(&s.f)
	return s.f
}
`,
			want: []string{"UnknownEffect f: unresolved call"},
		},
		{
			name:    "repeated field selections",
			ruleset: "explicitness",
			source: `package p

func f() int {
	var s T
	g(&s)
	x := s.f
	return x + s.g
}
`,
			want: []string{"UnknownEffect f: unresolved call"},
		},
		{
			name:    "nested selection",
			ruleset: "explicitness",
			source: `package p

func f() int {
	var s T
	g(&s)
	return s.f.g
}
`,
			want: []string{"UnknownEffect f: unresolved call"},
		},
		{
			name:    "index after address",
			ruleset: "explicitness",
			source: `package p

func f() int {
	var s T
	g(&s)
	return s[0]
}
`,
			want: []string{"UnknownEffect f: unresolved call"},
		},
		{
			name:    "read before address-taking",
			ruleset: "explicitness",
			source: `package p

func f() int {
	var s T
	x := s.f
	g(&s)
	return x
}
`,
			want: []string{"UnknownEffect f: unresolved call"},
		},
		{
			name:    "address-taking without a later selection",
			ruleset: "explicitness",
			source: `package p

func f() {
	var s T
	g(&s)
}
`,
			want: []string{"UnknownEffect f: unresolved call"},
		},
		{
			name:    "known package write survives",
			ruleset: "explicitness",
			source: `package p

var global int

func f() int {
	var s T
	g(&s)
	global = 1
	return s.f
}
`,
			want: []string{
				"UnknownEffect f: unresolved call",
				"ImplicitOutput f: package variable global",
			},
		},
		{
			name:    "parameter alias write survives",
			ruleset: "explicitness",
			source: `package p

func f(items []int) {
	var dst []int
	g(&dst, items)
	dst[0] = 1
}
`,
			want: []string{
				"UnknownEffect f: unresolved call",
				"ImplicitOutput f: write through parameter items",
			},
		},
		{
			name:    "real source call resolution",
			ruleset: "explicitness",
			path:    filepath.Join("..", "model", "call_resolution.go"),
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			code, findings, errOut := runAddressChild(t, test.ruleset, test.source, test.path)
			if strings.Contains(errOut, "stack overflow") || strings.Contains(errOut, "fatal error") {
				t.Fatalf("runtime fatal, exit %d\n%s", code, errOut)
			}
			if code != ExitSuccess && code != ExitViolation {
				t.Fatalf("exit = %d, stderr = %q", code, errOut)
			}
			if test.clean {
				if code != ExitSuccess || len(findings) != 0 {
					t.Fatalf("exit = %d, findings = %q, want clean", code, findings)
				}
				return
			}
			if test.want != nil {
				assertFindings(t, findings, test.want...)
			}
		})
	}
}

func runAddressChild(t *testing.T, ruleset, source, path string) (int, []string, string) {
	t.Helper()
	if path == "" {
		dir := t.TempDir()
		path = filepath.Join(dir, "p.go")
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0])
	cmd.Env = append(os.Environ(),
		"MESSGO_ADDR_CHILD=1",
		"MESSGO_ADDR_PATH="+path,
		"MESSGO_ADDR_RULES="+ruleset,
		"GOMAXPROCS=1",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	errOut := stderr.String()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("timed out after 8s; stderr = %q", errOut)
	}
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			t.Fatalf("run child: %v\n%s", err, errOut)
		}
	}
	if strings.Contains(errOut, "stack overflow") || strings.Contains(errOut, "fatal error") {
		return code, nil, errOut
	}
	findings := explicitnessFindings(t, stdout.String())
	return code, findings, errOut
}

func explicitnessFindings(t *testing.T, out string) []string {
	t.Helper()
	var report struct {
		Files []struct {
			File       string `json:"file"`
			Violations []struct {
				BeginLine   int    `json:"beginLine"`
				Function    string `json:"function"`
				Class       string `json:"class"`
				Method      string `json:"method"`
				Description string `json:"description"`
				Rule        string `json:"rule"`
			} `json:"violations"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}
	var got []string
	for _, f := range report.Files {
		for _, v := range f.Violations {
			m := explicitnessDetail.FindStringSubmatch(v.Description)
			if m == nil {
				continue
			}
			name := v.Function
			if v.Method != "" {
				name = v.Class + "." + v.Method
			}
			got = append(got, fmt.Sprintf("%s %s: %s", v.Rule, name, m[1]))
		}
	}
	sort.Strings(got)
	return got
}
