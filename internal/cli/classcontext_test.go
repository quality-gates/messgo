package cli

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const typeNameFixture = `package replay

type Fo struct{ v int }

type bad_name struct{ w int }

type Io interface{ M() }

type bad_iface interface{ N() }
`

const typeNameRuleset = `<ruleset name="cls">
  <rule ref="rulesets/naming.xml/ShortClassName"/>
  <rule ref="rulesets/naming.xml/LongClassName"><properties><property name="maximum" value="2"/></properties></rule>
  <rule ref="rulesets/controversial.xml/CamelCaseClassName"/>
  <rule ref="rulesets/codesize.xml/TooManyFields"><properties><property name="maxfields" value="0"/></properties></rule>
</ruleset>
`

// typeNameFindings is every expected "line rule class method function" entry
// for typeNameFixture, in report order.
var typeNameFindings = []string{
	"3 ShortClassName Fo  ",
	"3 TooManyFields Fo  ",
	"5 LongClassName bad_name  ",
	"5 CamelCaseClassName bad_name  ",
	"5 TooManyFields bad_name  ",
	"7 ShortClassName Io  ",
	"9 LongClassName bad_iface  ",
	"9 CamelCaseClassName bad_iface  ",
}

func runTypeNameReport(t *testing.T, format string) string {
	t.Helper()
	dir := writePackage(t, map[string]string{"cls.go": typeNameFixture})
	rulesetPath := filepath.Join(t.TempDir(), "cls.xml")
	if err := os.WriteFile(rulesetPath, []byte(typeNameRuleset), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runMain(t, dir, format, rulesetPath)
	if code != ExitViolation {
		t.Fatalf("exit = %d, want %d; stderr = %q", code, ExitViolation, errOut)
	}
	return out
}

func assertTypeNameFindings(t *testing.T, got []string) {
	t.Helper()
	if len(got) != len(typeNameFindings) {
		t.Fatalf("findings = %q, want %q", got, typeNameFindings)
	}
	for i := range got {
		if got[i] != typeNameFindings[i] {
			t.Errorf("finding %d = %q, want %q", i, got[i], typeNameFindings[i])
		}
	}
}

func TestTypeNameRulesReportClassInJSON(t *testing.T) {
	var report struct {
		Files []struct {
			Violations []struct {
				BeginLine int    `json:"beginLine"`
				Rule      string `json:"rule"`
				Class     string `json:"class"`
				Method    string `json:"method"`
				Function  string `json:"function"`
			} `json:"violations"`
		} `json:"files"`
	}
	out := runTypeNameReport(t, "json")
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}
	var got []string
	for _, f := range report.Files {
		for _, v := range f.Violations {
			got = append(got, fmt.Sprintf("%d %s %s %s %s", v.BeginLine, v.Rule, v.Class, v.Method, v.Function))
		}
	}
	assertTypeNameFindings(t, got)
}

func TestTypeNameRulesReportClassInXML(t *testing.T) {
	var report struct {
		Files []struct {
			Violations []struct {
				BeginLine int    `xml:"beginline,attr"`
				Rule      string `xml:"rule,attr"`
				Class     string `xml:"class,attr"`
				Method    string `xml:"method,attr"`
				Function  string `xml:"function,attr"`
			} `xml:"violation"`
		} `xml:"file"`
	}
	out := runTypeNameReport(t, "xml")
	if err := xml.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}
	var got []string
	for _, f := range report.Files {
		for _, v := range f.Violations {
			got = append(got, fmt.Sprintf("%d %s %s %s %s", v.BeginLine, v.Rule, v.Class, v.Method, v.Function))
		}
	}
	assertTypeNameFindings(t, got)
}
