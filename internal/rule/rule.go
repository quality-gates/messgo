// Package rule defines the rule engine: the Rule interface, violations, rule
// sets, and the report. This mirrors PHPMD's PHPMD\Rule, PHPMD\RuleViolation,
// PHPMD\RuleSet and PHPMD\Report.
package rule

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/quality-gates/messgo/internal/model"
)

// Violation is a single reported rule violation (PHPMD RuleViolation).
type Violation struct {
	Rule        Rule
	File        string
	BeginLine   int
	EndLine     int
	Description string // rendered message
	Args        []any
	Method      string
	Class       string
	Package     string
	Function    string
	Priority    int
	RuleSetName string
}

// Rule is the interface every rule implements (PHPMD\Rule).
type Rule interface {
	Name() string
	Message() string
	Priority() int
	SetName() string
	ExternalURL() string
	Description() string
	// Since returns the phpmd version the analogous rule appeared in.
	Since() string
}

// Properties carries configurable rule properties parsed from ruleset XML.
type Properties map[string]string

func (p Properties) Int(key string, def int) int {
	if v, ok := p[key]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func (p Properties) Float(key string, def float64) float64 {
	if v, ok := p[key]; ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}

type DefaultBool = bool

func (p Properties) Bool(key string, def DefaultBool) bool {
	if v, ok := p[key]; ok {
		switch v {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return def
}

func (p Properties) String(key, def string) string {
	if v, ok := p[key]; ok {
		return v
	}
	return def
}

// Context is handed to each rule during analysis. Rules append violations via
// the helper methods, which mirror PHPMD's addViolation.
type Context struct {
	File       *model.File
	props      Properties
	violations *[]*Violation
	rule       Rule
}

func (c *Context) Props() Properties { return c.props }

// Report records a violation at begin/end lines, with message arguments
// substituted into the rule's message template. Use the typed Report* helpers
// when an artifact is available so class/method/function metadata is captured.
func (c *Context) Report(beginLine, endLine int, args ...any) {
	c.report(beginLine, endLine, "", "", "", args)
}

func (c *Context) report(beginLine, endLine int, class, method, function string, args []any) {
	v := &Violation{
		Rule:        c.rule,
		File:        c.File.Path,
		BeginLine:   beginLine,
		EndLine:     endLine,
		Args:        args,
		Class:       class,
		Method:      method,
		Function:    function,
		Priority:    c.rule.Priority(),
		RuleSetName: c.rule.SetName(),
		Package:     c.File.Package,
		Description: RenderMessage(c.rule.Message(), args),
	}
	*c.violations = append(*c.violations, v)
}

// ReportFunc records a violation against a function or method, capturing
// class/method/function names the way PHPMD does for XML/JSON output.
func (c *Context) ReportFunc(fn *model.Function, args ...any) {
	if fn.IsMethod() {
		c.report(fn.Line, fn.EndLine, fn.Receiver, fn.Name, "", args)
		return
	}
	c.report(fn.Line, fn.EndLine, "", "", fn.Name, args)
}

// ReportFuncAt is like ReportFunc but at a specific line span.
func (c *Context) ReportFuncAt(fn *model.Function, beginLine, endLine int, args ...any) {
	if fn.IsMethod() {
		c.report(beginLine, endLine, fn.Receiver, fn.Name, "", args)
		return
	}
	c.report(beginLine, endLine, "", "", fn.Name, args)
}

// ReportClass records a violation against a class.
func (c *Context) ReportClass(class *model.Class, args ...any) {
	c.report(class.Line, class.EndLine, class.Name, "", "", args)
}

// ReportInterface records a violation against an interface.
func (c *Context) ReportInterface(i *model.Interface, args ...any) {
	c.report(i.Line, i.EndLine, i.Name, "", "", args)
}

// Artifact-aware rule interfaces. A rule implements one or more of these to be
// invoked for the corresponding artifact type (PHPMD ClassAware, MethodAware,
// FunctionAware, InterfaceAware).

type ClassRule interface {
	Rule
	ApplyClass(c *Context, class *model.Class)
}

type InterfaceRule interface {
	Rule
	ApplyInterface(c *Context, iface *model.Interface)
}

type MethodRule interface {
	Rule
	ApplyMethod(c *Context, fn *model.Function)
}

type FunctionRule interface {
	Rule
	ApplyFunction(c *Context, fn *model.Function)
}

// FileRule applies once per file (for rules that scan raw syntax).
type FileRule interface {
	Rule
	ApplyFile(c *Context)
}

// placeholderRe matches {0}, {1}, ... message placeholders. Compiled once at
// package load because RenderMessage runs for every reported violation.
var placeholderRe = regexp.MustCompile(`\{(\d+)\}`)

// phpmdRegexRe matches PHPMD's delimited "(pattern)flags" property encoding.
var phpmdRegexRe = regexp.MustCompile(`^\((.*)\)([imsxu]*)$`)

// RenderMessage substitutes {0}, {1}, ... placeholders in a PHPMD message
// template with the provided args.
func RenderMessage(tmpl string, args []any) string {
	return placeholderRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		idx, _ := strconv.Atoi(m[1 : len(m)-1])
		if idx >= 0 && idx < len(args) {
			return toStr(args[idx])
		}
		return m
	})
}

func toStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		// PHPMD prints integral floats without a decimal point.
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}

// SortViolations orders violations by file then begin line, matching PHPMD's
// report ordering.
func SortViolations(vs []*Violation) {
	sort.SliceStable(vs, func(i, j int) bool {
		if vs[i].File != vs[j].File {
			return vs[i].File < vs[j].File
		}
		return vs[i].BeginLine < vs[j].BeginLine
	})
}

// CompileRegex compiles a PHPMD-style regex property of the form "(pattern)i"
// (trailing i flag). Returns nil if empty.
func CompileRegex(pat string) *regexp.Regexp {
	if pat == "" {
		return nil
	}
	// PHPMD encodes regexes as a delimited pattern plus trailing flags, e.g.
	// "(^(set|get|is|has|with))i". Translate to Go's (?i) inline-flag form.
	body, flags := pat, ""
	if m := phpmdRegexRe.FindStringSubmatch(pat); m != nil {
		body, flags = m[1], strings.ReplaceAll(m[2], "u", "")
	}
	if flags != "" {
		body = "(?" + flags + ")" + body
	}
	re, err := regexp.Compile(body)
	if err != nil {
		return nil
	}
	return re
}
