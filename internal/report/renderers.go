package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/quality-gates/messgo/internal/rule"
)

// ----- Text / ANSI --------------------------------------------------------

// TextRenderer reproduces PHPMD's column-aligned text renderer:
// {location}<pad>{ruleName}<pad>{description}. With Colored, the rule name is
// yellow and the description red (ANSI), matching the `ansi` format.
type TextRenderer struct{ Colored bool }

const columnSpacing = 2

func (t *TextRenderer) Render(w io.Writer, r *Report) error {
	longestLoc, longestRule := 0, 0
	type row struct{ loc, name, desc string }
	rows := make([]row, 0, len(r.Violations))
	for _, v := range r.Violations {
		loc := fmt.Sprintf("%s:%d", v.File, v.BeginLine)
		name := v.Rule.Name()
		if len(loc) > longestLoc {
			longestLoc = len(loc)
		}
		if len(name) > longestRule {
			longestRule = len(name)
		}
		rows = append(rows, row{loc, name, v.Description})
	}
	for _, rw := range rows {
		fmt.Fprint(w, rw.loc)
		fmt.Fprint(w, strings.Repeat(" ", longestLoc+columnSpacing-len(rw.loc)))
		fmt.Fprint(w, t.color(rw.name, "33"))
		fmt.Fprint(w, strings.Repeat(" ", longestRule+columnSpacing-len(rw.name)))
		fmt.Fprint(w, t.color(rw.desc, "31"))
		fmt.Fprint(w, "\n")
	}
	for _, e := range r.Errors {
		fmt.Fprintf(w, "%s\t-\t%s\n", e.File, e.Message)
	}
	return nil
}

func (t *TextRenderer) color(s, code string) string {
	if !t.Colored {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// ----- XML ----------------------------------------------------------------

// XMLRenderer reproduces PHPMD's PMD-compatible XML output.
type XMLRenderer struct{}

func (XMLRenderer) Render(w io.Writer, r *Report) error {
	fmt.Fprint(w, "<?xml version=\"1.0\" encoding=\"UTF-8\" ?>\n")
	fmt.Fprintf(w, "<pmd version=\"%s\" tool=\"messgo\" timestamp=\"%s\">\n", Version(), time.Now().Format(time.RFC3339))
	var curFile string
	open := false
	for _, v := range r.Violations {
		if v.File != curFile {
			if open {
				fmt.Fprint(w, "  </file>\n")
			}
			curFile = v.File
			fmt.Fprintf(w, "  <file name=\"%s\">\n", xmlEscape(curFile))
			open = true
		}
		fmt.Fprint(w, "    <violation")
		fmt.Fprintf(w, " beginline=\"%d\"", v.BeginLine)
		fmt.Fprintf(w, " endline=\"%d\"", v.EndLine)
		fmt.Fprintf(w, " rule=\"%s\"", xmlEscape(v.Rule.Name()))
		fmt.Fprintf(w, " ruleset=\"%s\"", xmlEscape(v.RuleSetName))
		maybeAttr(w, "package", v.Package)
		maybeAttr(w, "externalInfoUrl", v.Rule.ExternalURL())
		maybeAttr(w, "function", v.Function)
		maybeAttr(w, "class", v.Class)
		maybeAttr(w, "method", v.Method)
		fmt.Fprintf(w, " priority=\"%d\"", v.Priority)
		fmt.Fprint(w, ">\n")
		fmt.Fprintf(w, "      %s\n", xmlEscape(v.Description))
		fmt.Fprint(w, "    </violation>\n")
	}
	if open {
		fmt.Fprint(w, "  </file>\n")
	}
	for _, e := range r.Errors {
		fmt.Fprintf(w, "  <error filename=\"%s\" msg=\"%s\" />\n", xmlEscape(e.File), xmlEscape(e.Message))
	}
	fmt.Fprint(w, "</pmd>\n")
	return nil
}

func maybeAttr(w io.Writer, name, val string) {
	if strings.TrimSpace(val) == "" {
		return
	}
	fmt.Fprintf(w, " %s=\"%s\"", name, xmlEscape(val))
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#039;")
	return r.Replace(s)
}

// ----- JSON ---------------------------------------------------------------

// JSONRenderer reproduces PHPMD's JSON structure.
type JSONRenderer struct{}

type jsonReport struct {
	Version   string      `json:"version"`
	Package   string      `json:"package"`
	Timestamp string      `json:"timestamp"`
	Files     []jsonFile  `json:"files"`
	Errors    []jsonError `json:"errors,omitempty"`
}

type jsonFile struct {
	File       string          `json:"file"`
	Violations []jsonViolation `json:"violations"`
}

type jsonViolation struct {
	BeginLine       int    `json:"beginLine"`
	EndLine         int    `json:"endLine"`
	Package         string `json:"package"`
	Function        string `json:"function"`
	Class           string `json:"class"`
	Method          string `json:"method"`
	Description     string `json:"description"`
	Rule            string `json:"rule"`
	RuleSet         string `json:"ruleSet"`
	ExternalInfoURL string `json:"externalInfoUrl"`
	Priority        int    `json:"priority"`
}

type jsonError struct {
	FileName string `json:"fileName"`
	Message  string `json:"message"`
}

func (JSONRenderer) Render(w io.Writer, r *Report) error {
	rep := jsonReport{
		Version:   Version(),
		Package:   "messgo",
		Timestamp: time.Now().Format(time.RFC3339),
		Files:     []jsonFile{},
	}
	idx := map[string]int{}
	for _, v := range r.Violations {
		i, ok := idx[v.File]
		if !ok {
			i = len(rep.Files)
			idx[v.File] = i
			rep.Files = append(rep.Files, jsonFile{File: v.File})
		}
		rep.Files[i].Violations = append(rep.Files[i].Violations, jsonViolation{
			BeginLine:       v.BeginLine,
			EndLine:         v.EndLine,
			Package:         v.Package,
			Function:        v.Function,
			Class:           v.Class,
			Method:          v.Method,
			Description:     v.Description,
			Rule:            v.Rule.Name(),
			RuleSet:         v.RuleSetName,
			ExternalInfoURL: v.Rule.ExternalURL(),
			Priority:        v.Priority,
		})
	}
	for _, e := range r.Errors {
		rep.Errors = append(rep.Errors, jsonError{FileName: e.File, Message: e.Message})
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	enc.SetIndent("", "    ")
	return enc.Encode(rep)
}

// ----- GitHub Actions -----------------------------------------------------

// GitHubRenderer emits GitHub Actions workflow commands.
type GitHubRenderer struct{}

func (GitHubRenderer) Render(w io.Writer, r *Report) error {
	for _, v := range r.Violations {
		fmt.Fprintf(w, "::warning file=%s,line=%d,col=1::%s (%s)\n",
			v.File, v.BeginLine, v.Description, v.Rule.Name())
	}
	for _, e := range r.Errors {
		fmt.Fprintf(w, "::error file=%s::%s\n", e.File, e.Message)
	}
	return nil
}

// ----- GitLab Code Quality ------------------------------------------------

// GitLabRenderer emits the GitLab Code Quality JSON format.
type GitLabRenderer struct{}

func (GitLabRenderer) Render(w io.Writer, r *Report) error {
	type loc struct {
		Path  string `json:"path"`
		Lines struct {
			Begin int `json:"begin"`
		} `json:"lines"`
	}
	type entry struct {
		Type        string `json:"type"`
		CheckName   string `json:"check_name"`
		Description string `json:"description"`
		Fingerprint string `json:"fingerprint"`
		Severity    string `json:"severity"`
		Location    loc    `json:"location"`
	}
	entries := make([]entry, 0, len(r.Violations))
	for _, v := range r.Violations {
		var e entry
		e.Type = "issue"
		e.CheckName = v.Rule.Name()
		e.Description = v.Description
		e.Severity = gitlabSeverity(v.Priority)
		e.Fingerprint = fingerprint(v)
		e.Location.Path = v.File
		e.Location.Lines.Begin = v.BeginLine
		entries = append(entries, e)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "    ")
	return enc.Encode(entries)
}

func gitlabSeverity(priority int) string {
	switch priority {
	case 1:
		return "blocker"
	case 2:
		return "critical"
	case 3:
		return "major"
	case 4:
		return "minor"
	default:
		return "info"
	}
}

func fingerprint(v *rule.Violation) string {
	return fmt.Sprintf("%x", fmt.Appendf(nil, "%s:%d:%s", v.File, v.BeginLine, v.Rule.Name()))
}

// ----- Checkstyle ---------------------------------------------------------

// CheckStyleRenderer emits Checkstyle XML.
type CheckStyleRenderer struct{}

func (CheckStyleRenderer) Render(w io.Writer, r *Report) error {
	fmt.Fprint(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	fmt.Fprintf(w, "<checkstyle version=\"%s\">\n", Version())
	var curFile string
	open := false
	for _, v := range r.Violations {
		if v.File != curFile {
			if open {
				fmt.Fprint(w, "  </file>\n")
			}
			curFile = v.File
			fmt.Fprintf(w, "  <file name=\"%s\">\n", xmlEscape(curFile))
			open = true
		}
		fmt.Fprintf(w, "    <error line=\"%d\" column=\"1\" severity=\"%s\" message=\"%s\" source=\"%s\"/>\n",
			v.BeginLine, checkstyleSeverity(v.Priority), xmlEscape(v.Description), xmlEscape(v.RuleSetName+"/"+v.Rule.Name()))
	}
	if open {
		fmt.Fprint(w, "  </file>\n")
	}
	fmt.Fprint(w, "</checkstyle>\n")
	return nil
}

func checkstyleSeverity(priority int) string {
	if priority <= 2 {
		return "error"
	}
	if priority == 3 {
		return "warning"
	}
	return "info"
}

// ----- SARIF --------------------------------------------------------------

// SARIFRenderer emits SARIF 2.1.0.
type SARIFRenderer struct{}

func (SARIFRenderer) Render(w io.Writer, r *Report) error {
	type artifactLoc struct {
		URI string `json:"uri"`
	}
	type region struct {
		StartLine int `json:"startLine"`
		EndLine   int `json:"endLine"`
	}
	type physLoc struct {
		ArtifactLocation artifactLoc `json:"artifactLocation"`
		Region           region      `json:"region"`
	}
	type location struct {
		PhysicalLocation physLoc `json:"physicalLocation"`
	}
	type result struct {
		RuleID  string `json:"ruleId"`
		Level   string `json:"level"`
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
		Locations []location `json:"locations"`
	}
	type driverRule struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		HelpURI          string `json:"helpUri,omitempty"`
		ShortDescription struct {
			Text string `json:"text"`
		} `json:"shortDescription"`
	}
	type driver struct {
		Name    string       `json:"name"`
		Version string       `json:"version"`
		Rules   []driverRule `json:"rules"`
	}
	type tool struct {
		Driver driver `json:"driver"`
	}
	type run struct {
		Tool    tool     `json:"tool"`
		Results []result `json:"results"`
	}
	type sarif struct {
		Schema  string `json:"$schema"`
		Version string `json:"version"`
		Runs    []run  `json:"runs"`
	}

	seen := map[string]bool{}
	var rules []driverRule
	var results []result
	for _, v := range r.Violations {
		id := v.Rule.Name()
		if !seen[id] {
			seen[id] = true
			var dr driverRule
			dr.ID = id
			dr.Name = id
			dr.HelpURI = v.Rule.ExternalURL()
			dr.ShortDescription.Text = strings.TrimSpace(v.Rule.Description())
			rules = append(rules, dr)
		}
		var res result
		res.RuleID = id
		res.Level = sarifLevel(v.Priority)
		res.Message.Text = v.Description
		var l location
		l.PhysicalLocation.ArtifactLocation.URI = v.File
		l.PhysicalLocation.Region.StartLine = v.BeginLine
		l.PhysicalLocation.Region.EndLine = v.EndLine
		res.Locations = []location{l}
		results = append(results, res)
	}
	doc := sarif{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []run{{
			Tool:    tool{Driver: driver{Name: "messgo", Version: Version(), Rules: rules}},
			Results: results,
		}},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func sarifLevel(priority int) string {
	if priority <= 2 {
		return "error"
	}
	return "warning"
}

// ----- HTML ---------------------------------------------------------------

// HTMLRenderer emits a simple HTML table report.
type HTMLRenderer struct{}

func (HTMLRenderer) Render(w io.Writer, r *Report) error {
	fmt.Fprint(w, "<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"><title>messgo report</title></head><body>\n")
	fmt.Fprint(w, "<h1>messgo report</h1>\n")
	var curFile string
	open := false
	for _, v := range r.Violations {
		if v.File != curFile {
			if open {
				fmt.Fprint(w, "</table>\n")
			}
			curFile = v.File
			fmt.Fprintf(w, "<h2>%s</h2>\n<table border=\"1\" cellspacing=\"0\" cellpadding=\"3\">\n", htmlEscape(curFile))
			fmt.Fprint(w, "<tr><th>Line</th><th>Rule</th><th>Description</th></tr>\n")
			open = true
		}
		fmt.Fprintf(w, "<tr><td>%d</td><td>%s</td><td>%s</td></tr>\n",
			v.BeginLine, htmlEscape(v.Rule.Name()), htmlEscape(v.Description))
	}
	if open {
		fmt.Fprint(w, "</table>\n")
	}
	fmt.Fprint(w, "</body></html>\n")
	return nil
}

func htmlEscape(s string) string { return xmlEscape(s) }
