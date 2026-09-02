// Package runner orchestrates discovery, parsing and analysis of Go sources.
package runner

import (
	"go/ast"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/report"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/util"
)

// Options configures a run.
type Options struct {
	Paths       []string // files or directories to scan
	RuleSets    []*rule.RuleSet
	Suffixes    []string // file extensions to include (default ".go")
	Exclude     []string // path substrings to skip
	IgnoreTests bool     // skip *_test.go files
}

// Run discovers files, parses and analyzes them, and returns a Report.
func Run(opts Options) (*report.Report, error) {
	if len(opts.Suffixes) == 0 {
		opts.Suffixes = []string{".go"}
	}
	files, err := discover(opts)
	if err != nil {
		return nil, err
	}
	rep := &report.Report{}
	parsed := parseFiles(files, rep)
	annotatePackages(parsed)
	for _, file := range parsed {
		vs := rule.Analyze(file, opts.RuleSets)
		rep.Violations = append(rep.Violations, vs...)
	}
	rule.SortViolations(rep.Violations)
	return rep, nil
}

// parseFiles parses every path, recording parse failures on the report and
// returning the files that parsed successfully.
func parseFiles(files []string, rep *report.Report) []*model.File {
	var parsed []*model.File
	for _, path := range files {
		file, err := model.Parse(path)
		if err != nil {
			rep.Errors = append(rep.Errors, report.ProcessingError{File: path, Message: err.Error()})
			continue
		}
		parsed = append(parsed, file)
	}
	return parsed
}

// annotatePackages groups files by directory (a Go package lives in one
// directory) and records, on every file, the set of package-level variables
// mutated anywhere in that package — enabling cross-file analysis.
type packageKey struct {
	dir  string
	name string
}

func annotatePackages(parsed []*model.File) {
	byPkg := map[packageKey][]*model.File{}
	for _, f := range parsed {
		byPkg[packageKeyOf(f)] = append(byPkg[packageKeyOf(f)], f)
	}
	for _, group := range byPkg {
		asts := make([]*ast.File, len(group))
		for i, f := range group {
			asts[i] = f.Syntax
		}
		mutated := util.MutatedGlobalNames(asts)
		var pkgClasses []*model.Class
		for _, f := range group {
			pkgClasses = append(pkgClasses, f.Classes...)
		}
		for _, f := range group {
			f.MutatedGlobals = mutated
			f.PackageClasses = pkgClasses
		}
	}
}

func packageKeyOf(f *model.File) packageKey {
	dir := filepath.Dir(f.Path)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	return packageKey{dir: dir, name: f.Package}
}

func walkDirFunc(opts Options, add func(string)) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !shouldIncludeFile(path, opts) {
			return nil
		}
		add(path)
		return nil
	}
}

func discover(opts Options) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		abs, _ := filepath.Abs(p)
		if seen[abs] {
			return
		}
		seen[abs] = true
		out = append(out, p)
	}
	for _, p := range opts.Paths {
		root := discoveryRoot(p)
		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if !shouldIncludeFile(root, opts) {
				continue
			}
			add(root)
			continue
		}
		err = filepath.WalkDir(root, walkDirFunc(opts, add))
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func discoveryRoot(p string) string {
	if filepath.Base(p) != "..." {
		return p
	}
	return filepath.Dir(p)
}

func shouldIncludeFile(path string, opts Options) bool {
	if !hasSuffix(path, opts.Suffixes) {
		return false
	}
	if opts.IgnoreTests && strings.HasSuffix(path, "_test.go") {
		return false
	}
	return !isExcluded(path, opts.Exclude)
}

func shouldSkipDir(name string) bool {
	switch name {
	case "vendor", "node_modules", ".git":
		return true
	}
	return false
}

func hasSuffix(path string, suffixes []string) bool {
	for _, s := range suffixes {
		if strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}

func isExcluded(path string, exclude []string) bool {
	for _, e := range exclude {
		if e != "" && strings.Contains(path, e) {
			return true
		}
	}
	return false
}
