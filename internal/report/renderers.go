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

type checkedWriter struct {
	io.Writer
	err error
}

func (w *checkedWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.Writer.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = err
	}
	return n, err
}

func (t *TextRenderer) Render(w io.Writer, r *Report) error {
	out := &checkedWriter{Writer: w}
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
		fmt.Fprint(out, rw.loc)
		fmt.Fprint(out, strings.Repeat(" ", longestLoc+columnSpacing-len(rw.loc)))
		fmt.Fprint(out, t.color(rw.name, "33"))
		fmt.Fprint(out, strings.Repeat(" ", longestRule+columnSpacing-len(rw.name)))
		fmt.Fprint(out, t.color(rw.desc, "31"))
		fmt.Fprint(out, "\n")
	}
	for _, e := range r.Errors {
		fmt.Fprintf(out, "%s\t-\t%s\n", e.File, e.Message)
	}
	return out.err
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
	out := &checkedWriter{Writer: w}
	fmt.Fprint(out, "<?xml version=\"1.0\" encoding=\"UTF-8\" ?>\n")
	fmt.Fprintf(out, "<pmd version=\"%s\" tool=\"messgo\" timestamp=\"%s\">\n", Version, time.Now().Format(time.RFC3339))
	var curFile string
	open := false
	for _, v := range r.Violations {
		if v.File != curFile {
			if open {
				fmt.Fprint(out, "  </file>\n")
			}
			curFile = v.File
			fmt.Fprintf(out, "  <file name=\"%s\">\n", xmlEscape(curFile))
			open = true
		}
		fmt.Fprint(out, "    <violation")
		fmt.Fprintf(out, " beginline=\"%d\"", v.BeginLine)
		fmt.Fprintf(out, " endline=\"%d\"", v.EndLine)
		fmt.Fprintf(out, " rule=\"%s\"", xmlEscape(v.Rule.Name()))
		fmt.Fprintf(out, " ruleset=\"%s\"", xmlEscape(v.RuleSetName))
		maybeAttr(out, "package", v.Package)
		maybeAttr(out, "externalInfoUrl", v.Rule.ExternalURL())
		maybeAttr(out, "function", v.Function)
		maybeAttr(out, "class", v.Class)
		maybeAttr(out, "method", v.Method)
		fmt.Fprintf(out, " priority=\"%d\"", v.Priority)
		fmt.Fprint(out, ">\n")
		fmt.Fprintf(out, "      %s\n", xmlEscape(v.Description))
		fmt.Fprint(out, "    </violation>\n")
	}
	if open {
		fmt.Fprint(out, "  </file>\n")
	}
	for _, e := range r.Errors {
		fmt.Fprintf(out, "  <error filename=\"%s\" msg=\"%s\" />\n", xmlEscape(e.File), xmlEscape(e.Message))
	}
	fmt.Fprint(out, "</pmd>\n")
	return out.err
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
	out := &checkedWriter{Writer: w}
	rep := jsonReport{
		Version:   Version,
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
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(true)
	enc.SetIndent("", "    ")
	if err := enc.Encode(rep); err != nil {
		return err
	}
	return out.err
}

// ----- GitHub Actions -----------------------------------------------------

// GitHubRenderer emits GitHub Actions workflow commands.
type GitHubRenderer struct{}

func (GitHubRenderer) Render(w io.Writer, r *Report) error {
	out := &checkedWriter{Writer: w}
	for _, v := range r.Violations {
		fmt.Fprintf(out, "::warning file=%s,line=%d,col=1::%s (%s)\n",
			githubEscapeProperty(v.File), v.BeginLine,
			githubEscape(v.Description), githubEscape(v.Rule.Name()))
	}
	for _, e := range r.Errors {
		fmt.Fprintf(out, "::error file=%s::%s\n", githubEscapeProperty(e.File), githubEscape(e.Message))
	}
	return out.err
}

func githubEscape(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	return strings.ReplaceAll(s, "\n", "%0A")
}

func githubEscapeProperty(s string) string {
	s = githubEscape(s)
	s = strings.ReplaceAll(s, ":", "%3A")
	return strings.ReplaceAll(s, ",", "%2C")
}

// ----- GitLab Code Quality ------------------------------------------------

// GitLabRenderer emits the GitLab Code Quality JSON format.
type GitLabRenderer struct{}

func (GitLabRenderer) Render(w io.Writer, r *Report) error {
	out := &checkedWriter{Writer: w}
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
	for _, errItem := range r.Errors {
		var e entry
		e.Type = "issue"
		e.CheckName = "parse-error"
		e.Description = errItem.Message
		e.Severity = "blocker"
		e.Fingerprint = fmt.Sprintf("%x", fmt.Appendf(nil, "%s:%s", errItem.File, errItem.Message))
		e.Location.Path = errItem.File
		entries = append(entries, e)
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "    ")
	if err := enc.Encode(entries); err != nil {
		return err
	}
	return out.err
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
	out := &checkedWriter{Writer: w}
	fmt.Fprint(out, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	fmt.Fprintf(out, "<checkstyle version=\"%s\">\n", Version)
	var curFile string
	open := false
	for _, v := range r.Violations {
		if v.File != curFile {
			if open {
				fmt.Fprint(out, "  </file>\n")
			}
			curFile = v.File
			fmt.Fprintf(out, "  <file name=\"%s\">\n", xmlEscape(curFile))
			open = true
		}
		fmt.Fprintf(out, "    <error line=\"%d\" column=\"1\" severity=\"%s\" message=\"%s\" source=\"%s\"/>\n",
			v.BeginLine, checkstyleSeverity(v.Priority), xmlEscape(v.Description), xmlEscape(v.RuleSetName+"/"+v.Rule.Name()))
	}
	if open {
		fmt.Fprint(out, "  </file>\n")
	}
	for _, e := range r.Errors {
		fmt.Fprintf(out, "  <file name=\"%s\">\n", xmlEscape(e.File))
		fmt.Fprintf(out, "    <error line=\"0\" column=\"1\" severity=\"error\" message=\"%s\" source=\"messgo/parse-error\"/>\n", xmlEscape(e.Message))
		fmt.Fprint(out, "  </file>\n")
	}
	fmt.Fprint(out, "</checkstyle>\n")
	return out.err
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

// SARIF 2.1.0 document types.
type (
	sarifArtifactLoc struct {
		URI string `json:"uri"`
	}
	sarifRegion struct {
		StartLine int `json:"startLine"`
		EndLine   int `json:"endLine"`
	}
	sarifPhysLoc struct {
		ArtifactLocation sarifArtifactLoc `json:"artifactLocation"`
		Region           *sarifRegion     `json:"region,omitempty"`
	}
	sarifLocation struct {
		PhysicalLocation sarifPhysLoc `json:"physicalLocation"`
	}
	sarifResult struct {
		RuleID  string `json:"ruleId"`
		Level   string `json:"level"`
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
		Locations []sarifLocation `json:"locations"`
	}
	sarifDriverRule struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		HelpURI          string `json:"helpUri,omitempty"`
		ShortDescription struct {
			Text string `json:"text"`
		} `json:"shortDescription"`
	}
	sarifDriver struct {
		Name    string            `json:"name"`
		Version string            `json:"version"`
		Rules   []sarifDriverRule `json:"rules"`
	}
	sarifTool struct {
		Driver sarifDriver `json:"driver"`
	}
	sarifRun struct {
		Tool    sarifTool     `json:"tool"`
		Results []sarifResult `json:"results"`
	}
	sarifDoc struct {
		Schema  string     `json:"$schema"`
		Version string     `json:"version"`
		Runs    []sarifRun `json:"runs"`
	}
)

// SARIFRenderer emits SARIF 2.1.0.
type SARIFRenderer struct{}

func (SARIFRenderer) Render(w io.Writer, r *Report) error {
	out := &checkedWriter{Writer: w}
	seen := map[string]bool{}
	rules := make([]sarifDriverRule, 0)
	results := make([]sarifResult, 0)
	for _, v := range r.Violations {
		id := v.Rule.Name()
		if !seen[id] {
			seen[id] = true
			var dr sarifDriverRule
			dr.ID = id
			dr.Name = id
			dr.HelpURI = v.Rule.ExternalURL()
			dr.ShortDescription.Text = strings.TrimSpace(v.Rule.Description())
			rules = append(rules, dr)
		}
		var res sarifResult
		res.RuleID = id
		res.Level = sarifLevel(v.Priority)
		res.Message.Text = v.Description
		var l sarifLocation
		l.PhysicalLocation.ArtifactLocation.URI = v.File
		if v.BeginLine > 0 {
			l.PhysicalLocation.Region = &sarifRegion{
				StartLine: v.BeginLine,
				EndLine:   v.EndLine,
			}
		}
		res.Locations = []sarifLocation{l}
		results = append(results, res)
	}
	for _, e := range r.Errors {
		var res sarifResult
		res.Level = "error"
		res.Message.Text = e.Message
		var loc sarifLocation
		loc.PhysicalLocation.ArtifactLocation.URI = e.File
		res.Locations = []sarifLocation{loc}
		results = append(results, res)
	}
	doc := sarifDoc{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool:    sarifTool{Driver: sarifDriver{Name: "messgo", Version: Version, Rules: rules}},
			Results: results,
		}},
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	return out.err
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
	out := &checkedWriter{Writer: w}
	fmt.Fprint(out, "<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"><title>messgo report</title></head><body>\n")
	fmt.Fprint(out, "<h1>messgo report</h1>\n")
	var curFile string
	open := false
	for _, v := range r.Violations {
		if v.File != curFile {
			if open {
				fmt.Fprint(out, "</table>\n")
			}
			curFile = v.File
			fmt.Fprintf(out, "<h2>%s</h2>\n<table border=\"1\" cellspacing=\"0\" cellpadding=\"3\">\n", htmlEscape(curFile))
			fmt.Fprint(out, "<tr><th>Line</th><th>Rule</th><th>Description</th></tr>\n")
			open = true
		}
		fmt.Fprintf(out, "<tr><td>%d</td><td>%s</td><td>%s</td></tr>\n",
			v.BeginLine, htmlEscape(v.Rule.Name()), htmlEscape(v.Description))
	}
	if open {
		fmt.Fprint(out, "</table>\n")
	}
	for _, e := range r.Errors {
		fmt.Fprintf(out, "<p>%s: %s</p>\n", htmlEscape(e.File), htmlEscape(e.Message))
	}
	fmt.Fprint(out, "</body></html>\n")
	return out.err
}

func htmlEscape(s string) string { return xmlEscape(s) }
