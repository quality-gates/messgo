package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"slices"
	"strings"

	"github.com/quality-gates/messgo/internal/model"
)

type caseSpec struct {
	root       byte
	left       string
	right      string
	rightFirst bool
}

type expectedSelection struct {
	method string
	uses   map[model.MemberKey]bool
}

type finding struct {
	caseID   string
	source   string
	expected string
	actual   string
}

type results struct {
	generated, valid, missing, ambiguous int
	callFailures, useFailures            int
	inventedMethods, wrongMethods        int
	missingMethods                       int
	callExamples, useExamples            []finding
}

func main() {
	caseID := flag.String("case", "", "check only the named generated case")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(1)
	}
	if err := check(*caseID); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func check(caseID string) error {
	var result results
	digest := sha256.New()
	for _, c := range cases() {
		if caseID != "" && caseID != c.id() {
			continue
		}
		result.generated++
		kind, err := selectorKind(c)
		if err != nil {
			return fmt.Errorf("%s: %w", c.id(), err)
		}
		switch kind {
		case "missing":
			result.missing++
			continue
		case "ambiguous":
			result.ambiguous++
			continue
		}
		result.valid++
		call, uses, err := compare(c)
		if err != nil {
			return fmt.Errorf("%s: %w", c.id(), err)
		}
		if call != nil {
			result.callFailures++
			switch {
			case call.expected == "<none>":
				result.inventedMethods++
			case call.actual == "<none>":
				result.missingMethods++
			default:
				result.wrongMethods++
			}
			fmt.Fprintf(digest, "%s call\n", c.id())
			if len(result.callExamples) < 2 {
				result.callExamples = append(result.callExamples, *call)
			}
		}
		if uses != nil {
			result.useFailures++
			fmt.Fprintf(digest, "%s uses\n", c.id())
			if len(result.useExamples) < 2 {
				result.useExamples = append(result.useExamples, *uses)
			}
		}
	}
	if result.generated == 0 {
		return fmt.Errorf("case %q was not generated", caseID)
	}
	if caseID != "" && result.valid == 0 {
		return fmt.Errorf("case %q has no unique selector", caseID)
	}
	if caseID == "" && (result.generated != 9126 || result.valid != 7620 || result.missing != 18 || result.ambiguous != 1488) {
		return fmt.Errorf("unexpected coverage: generated %d, checked %d, missing %d, ambiguous %d", result.generated, result.valid, result.missing, result.ambiguous)
	}
	fmt.Printf("Generated %d cases; checked %d; excluded %d missing and %d ambiguous selectors.\n", result.generated, result.valid, result.missing, result.ambiguous)
	fmt.Printf("Call target mismatches: %d; member-use mismatches: %d.\n", result.callFailures, result.useFailures)
	fmt.Printf("Call mismatches: %d field resolved as method, %d wrong method, %d missing method.\n", result.inventedMethods, result.wrongMethods, result.missingMethods)
	fmt.Printf("Mismatch fingerprint: %x\n", digest.Sum(nil))
	printFindings("Call target", result.callExamples)
	printFindings("Member uses", result.useExamples)
	if result.callFailures+result.useFailures != 0 {
		return errors.New("promoted selector verification failed")
	}
	return nil
}

func cases() []caseSpec {
	var all []caseSpec
	for leftDepth := 1; leftDepth <= 3; leftDepth++ {
		for _, left := range branches(leftDepth) {
			for rightDepth := 1; rightDepth <= 3; rightDepth++ {
				for _, right := range branches(rightDepth) {
					for _, root := range []byte{'0', 'M', 'F'} {
						for _, rightFirst := range []bool{false, true} {
							all = append(all, caseSpec{root, left, right, rightFirst})
						}
					}
				}
			}
		}
	}
	return all
}

func branches(depth int) []string {
	if depth == 0 {
		return []string{""}
	}
	var result []string
	for _, suffix := range branches(depth - 1) {
		for _, member := range []string{"0", "M", "F"} {
			result = append(result, member+suffix)
		}
	}
	return result
}

func (c caseSpec) id() string {
	order := "LR"
	if c.rightFirst {
		order = "RL"
	}
	return fmt.Sprintf("root%c-L%s-R%s-%s", c.root, c.left, c.right, order)
}

func (c caseSpec) source(withCall bool) string {
	var source strings.Builder
	source.WriteString("package p\n\n")
	first, second := "L1", "R1"
	if c.rightFirst {
		first, second = second, first
	}
	fmt.Fprintf(&source, "type Root struct { %s; %s;", first, second)
	writeField(&source, c.root)
	source.WriteString(" }\n")
	writeBranch(&source, "L", c.left)
	writeBranch(&source, "R", c.right)
	writeMethod(&source, "Root", c.root)
	if withCall {
		source.WriteString("func probe(x Root) int { return x.m() }\n")
	}
	return source.String()
}

func writeBranch(source *strings.Builder, prefix, members string) {
	for i, member := range []byte(members) {
		name := fmt.Sprintf("%s%d", prefix, i+1)
		fmt.Fprintf(source, "type %s struct {", name)
		if i+1 < len(members) {
			fmt.Fprintf(source, " %s%d;", prefix, i+2)
		}
		writeField(source, member)
		source.WriteString(" }\n")
		writeMethod(source, name, member)
	}
}

func writeField(source *strings.Builder, member byte) {
	if member == 'F' {
		source.WriteString(" m func() int;")
	}
}

func writeMethod(source *strings.Builder, name string, member byte) {
	if member == 'M' {
		fmt.Fprintf(source, "func (%s) m() int { return 1 }\n", name)
	}
}

func selectorKind(c caseSpec) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "decls.go", c.source(false), 0)
	if err != nil {
		return "", fmt.Errorf("parse declarations: %w", err)
	}
	pkg, err := new(types.Config).Check("p", fset, []*ast.File{file}, nil)
	if err != nil {
		return "", fmt.Errorf("type-check declarations: %w", err)
	}
	root := pkg.Scope().Lookup("Root")
	if root == nil {
		return "", errors.New("generated Root type is missing")
	}
	object, index, _ := types.LookupFieldOrMethod(root.Type(), true, pkg, "m")
	if object != nil {
		return "unique", nil
	}
	if index != nil {
		return "ambiguous", nil
	}
	return "missing", nil
}

func compare(c caseSpec) (*finding, *finding, error) {
	source := c.source(true)
	file, err := model.ParseSource("fixture.go", []byte(source))
	if err != nil {
		return nil, nil, fmt.Errorf("parse model source: %w", err)
	}
	var probe *model.Function
	for _, fn := range file.Functions {
		if fn.Name == "probe" {
			if probe != nil {
				return nil, nil, errors.New("multiple probe functions")
			}
			probe = fn
		}
	}
	if probe == nil {
		return nil, nil, errors.New("probe function is missing")
	}
	var calls []*ast.CallExpr
	ast.Inspect(probe.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			calls = append(calls, call)
		}
		return true
	})
	if len(calls) != 1 {
		return nil, nil, fmt.Errorf("probe contains %d calls, want 1", len(calls))
	}
	selector, ok := calls[0].Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "m" {
		return nil, nil, errors.New("probe call is not a selector for m")
	}
	info := &types.Info{Selections: map[*ast.SelectorExpr]*types.Selection{}}
	if _, err := new(types.Config).Check("p", file.Fset, []*ast.File{file.Syntax}, info); err != nil {
		return nil, nil, fmt.Errorf("type-check probe: %w", err)
	}
	selection := info.Selections[selector]
	if selection == nil || len(info.Selections) != 1 {
		return nil, nil, errors.New("type checker did not return exactly one probe selection")
	}
	expected, err := selectionOracle(selection)
	if err != nil {
		return nil, nil, err
	}
	actualMethod := "<none>"
	if fn := model.ResolveCall(probe, calls[0]); fn != nil {
		actualMethod = fn.Receiver + "." + fn.Name
	}
	var callFinding *finding
	if actualMethod != expected.method {
		callFinding = &finding{c.id(), source, expected.method, actualMethod}
	}
	actualUses := model.SelectedMemberUses(file)
	if c.root == '0' {
		delete(actualUses, model.MemberKey{Type: "Root", Name: "m"})
	}
	var useFinding *finding
	expectedUseNames, actualUseNames := formatUses(expected.uses), formatUses(actualUses)
	if expectedUseNames != actualUseNames {
		useFinding = &finding{c.id(), source, expectedUseNames, actualUseNames}
	}
	return callFinding, useFinding, nil
}

func selectionOracle(selection *types.Selection) (expectedSelection, error) {
	named, ok := selection.Recv().(*types.Named)
	if !ok {
		return expectedSelection{}, errors.New("selection receiver is not a named type")
	}
	indexes := selection.Index()
	if len(indexes) == 0 {
		return expectedSelection{}, errors.New("selection path is empty")
	}
	uses := map[model.MemberKey]bool{}
	for _, index := range indexes[:len(indexes)-1] {
		structure, ok := named.Underlying().(*types.Struct)
		if !ok || index >= structure.NumFields() {
			return expectedSelection{}, errors.New("selection path is not a struct field")
		}
		field := structure.Field(index)
		if !field.Embedded() {
			return expectedSelection{}, errors.New("selection path crosses a non-embedded field")
		}
		uses[model.MemberKey{Type: named.Obj().Name(), Name: field.Name()}] = true
		named, ok = field.Type().(*types.Named)
		if !ok {
			return expectedSelection{}, errors.New("embedded field is not a named type")
		}
	}
	method := "<none>"
	switch object := selection.Obj().(type) {
	case *types.Func:
		signature, ok := object.Type().(*types.Signature)
		if !ok || signature.Recv() == nil {
			return expectedSelection{}, errors.New("selected method has no receiver")
		}
		owner, ok := signature.Recv().Type().(*types.Named)
		if !ok || owner.Obj().Name() != named.Obj().Name() {
			return expectedSelection{}, errors.New("selected method owner disagrees with path")
		}
		method = owner.Obj().Name() + "." + object.Name()
	case *types.Var:
		structure, ok := named.Underlying().(*types.Struct)
		last := indexes[len(indexes)-1]
		if !ok || last >= structure.NumFields() || structure.Field(last) != object {
			return expectedSelection{}, errors.New("selected field disagrees with path")
		}
	default:
		return expectedSelection{}, fmt.Errorf("unsupported selected object %T", object)
	}
	uses[model.MemberKey{Type: named.Obj().Name(), Name: selection.Obj().Name()}] = true
	return expectedSelection{method, uses}, nil
}

func formatUses(uses map[model.MemberKey]bool) string {
	var names []string
	for key, selected := range uses {
		if selected {
			names = append(names, key.Type+"."+key.Name)
		}
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

func printFindings(property string, findings []finding) {
	for _, f := range findings {
		fmt.Printf("\n%s: %s\n%sExpected: %s\nActual:   %s\n", property, f.caseID, f.source, f.expected, f.actual)
	}
}
