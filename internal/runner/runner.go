// Package runner orchestrates discovery, parsing and analysis of Go sources.
package runner

import (
	"go/ast"
	"go/token"
	"path/filepath"

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
	var patterns []string
	for _, set := range opts.RuleSets {
		if set != nil {
			patterns = append(patterns, set.ExcludePatterns...)
		}
	}
	files, err := DiscoverSources(DiscoveryOptions{
		Paths: opts.Paths, Suffixes: opts.Suffixes, Exclude: opts.Exclude,
		ExcludePatterns: patterns, IgnoreTests: opts.IgnoreTests,
	})
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
		annotatePackageGroup(group)
	}
}

func annotatePackageGroup(group []*model.File) {
	asts := make([]*ast.File, len(group))
	for i, f := range group {
		asts[i] = f.Syntax
	}
	mutated := util.MutatedGlobalNames(asts)
	var (
		pkgClasses    []*model.Class
		pkgInterfaces []*model.Interface
		pkgFunctions  []*model.Function
	)
	classByName := map[string]*model.Class{}
	for _, f := range group {
		pkgClasses = append(pkgClasses, f.Classes...)
		for _, c := range f.Classes {
			classByName[c.Name] = c
		}
		pkgInterfaces = append(pkgInterfaces, f.Interfaces...)
		pkgFunctions = append(pkgFunctions, f.Functions...)
	}
	pkgTypeDefs := collectPackageTypeExprs(group)
	pkgTypeIndex := model.NewPackageTypeIndex(pkgClasses, pkgInterfaces)
	packageMapTypes := collectPackageMapTypes(pkgTypeDefs)
	for _, f := range group {
		f.PackageClasses = pkgClasses
		f.PackageInterfaces = pkgInterfaces
		f.PackageTypeIndex = pkgTypeIndex
		f.PackageMapTypes = packageMapTypes
		f.PackageTypeDefs = pkgTypeDefs
		f.PackageFunctions = pkgFunctions
	}
	attachPackageMethods(group, classByName, util.TypeAliasNames(asts))
	sharedGlobals := util.SharedGlobalNames(asts, pkgTypeDefs)
	globalSharings := util.GlobalSharings(asts, pkgTypeDefs)
	globalTypes := util.GlobalValueTypes(asts, pkgTypeDefs)
	pkgMemberSelections := collectPackageMemberSelections(group)
	for _, f := range group {
		f.MutatedGlobals = mutated
		f.SharedGlobals = sharedGlobals
		f.PackageGlobalSharings = globalSharings
		f.PackageGlobalTypes = globalTypes
		f.PackageMemberSelections = pkgMemberSelections
	}
}

func collectPackageTypeExprs(group []*model.File) map[string]ast.Expr {
	typeExprs := map[string]ast.Expr{}
	for _, f := range group {
		for _, decl := range f.Syntax.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					typeExprs[typeSpec.Name.Name] = typeSpec.Type
				}
			}
		}
	}
	return typeExprs
}

func collectPackageMapTypes(typeExprs map[string]ast.Expr) map[string]bool {
	mapTypes := map[string]bool{}
	for name := range typeExprs {
		if resolvesToMapType(typeExprs, name) {
			mapTypes[name] = true
		}
	}
	return mapTypes
}

// resolvesToMapType follows a chain of package type names, through generic
// instantiations, to report whether name denotes a map type.
func resolvesToMapType(typeExprs map[string]ast.Expr, name string) bool {
	seen := map[string]bool{}
	for !seen[name] {
		seen[name] = true
		switch t := util.UnwrapTypeInstance(typeExprs[name]).(type) {
		case *ast.MapType:
			return true
		case *ast.Ident:
			name = t.Name
		default:
			return false
		}
	}
	return false
}

func attachPackageMethods(group []*model.File, classByName map[string]*model.Class, aliases map[string]string) {
	for _, f := range group {
		for _, fn := range f.AllFuncs {
			if !fn.IsMethod() {
				continue
			}
			c := classByName[util.ResolveTypeAlias(aliases, fn.Receiver)]
			if c == nil {
				continue
			}
			fn.Class = c
			if !hasMethod(c.Methods, fn) {
				c.Methods = append(c.Methods, fn)
			}
		}
	}
}

func hasMethod(methods []*model.Function, fn *model.Function) bool {
	for _, m := range methods {
		if m == fn {
			return true
		}
	}
	return false
}

func collectPackageMemberSelections(group []*model.File) map[model.MemberKey]bool {
	members := map[model.MemberKey]bool{}
	for _, f := range group {
		for key := range model.SelectedMemberUses(f) {
			members[key] = true
		}
	}
	return members
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
