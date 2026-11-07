package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type branch struct {
	present   bool
	statement string
}

type testCase struct {
	name       string
	initialA   string
	initialB   string
	then       string
	otherwise  branch
	after      string
	writeAlias string
}

type observation struct {
	caseIndex int
	condition bool
	value     int
}

type jsonReport struct {
	Files []struct {
		File       string `json:"file"`
		Violations []struct {
			Function string `json:"function"`
			Rule     string `json:"rule"`
		} `json:"violations"`
	} `json:"files"`
	Errors []json.RawMessage `json:"errors"`
}

func cases() []testCase {
	initialA := []string{"p", "[]int{0}"}
	initialB := []string{"p", "a", "[]int{0}"}
	arms := []string{"", "a = p", "a = b", "a = []int{0}", "b = p", "b = a", "b = []int{0}"}
	elseArms := []branch{{}}
	for _, statement := range arms {
		elseArms = append(elseArms, branch{present: true, statement: statement})
	}
	var all []testCase
	for _, a := range initialA {
		for _, b := range initialB {
			for _, then := range arms {
				for _, otherwise := range elseArms {
					for _, after := range []string{"", "a = b", "b = a"} {
						for _, target := range []string{"a", "b"} {
							all = append(all, testCase{
								name: fmt.Sprintf("Case%04d", len(all)), initialA: a, initialB: b,
								then: then, otherwise: otherwise, after: after, writeAlias: target,
							})
						}
					}
				}
			}
		}
	}
	return all
}

func renderCase(c testCase) string {
	var out strings.Builder
	fmt.Fprintf(&out, "func %s(p []int, flag bool) {\n", c.name)
	fmt.Fprintf(&out, "\ta := %s\n\tb := %s\n\t_ = a\n\t_ = b\n", c.initialA, c.initialB)
	fmt.Fprintf(&out, "\tif flag {\n\t\t%s\n\t}", c.then)
	if c.otherwise.present {
		fmt.Fprintf(&out, " else {\n\t\t%s\n\t}", c.otherwise.statement)
	}
	fmt.Fprintf(&out, "\n\t%s\n\t%s[0] = 1\n}\n", c.after, c.writeAlias)
	return out.String()
}

func writePrograms(dir string, all []testCase) (string, string, error) {
	var source, driver strings.Builder
	source.WriteString("package main\n\n")
	driver.WriteString("package main\n\nimport \"fmt\"\n\nfunc main() {\n")
	driver.WriteString("functions := []func([]int, bool){\n")
	for _, c := range all {
		source.WriteString(renderCase(c))
		source.WriteByte('\n')
		fmt.Fprintf(&driver, "%s,\n", c.name)
	}
	driver.WriteString("}\nfor i, fn := range functions {\n")
	driver.WriteString("for _, flag := range []bool{false, true} {\n")
	driver.WriteString("p := []int{0}\nfn(p, flag)\nfmt.Printf(\"%d %t %d\\n\", i, flag, p[0])\n")
	driver.WriteString("}\n}\n}\n")
	sourcePath := filepath.Join(dir, "cases.go")
	driverPath := filepath.Join(dir, "driver.go")
	if err := os.WriteFile(sourcePath, []byte(source.String()), 0o600); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(driverPath, []byte(driver.String()), 0o600); err != nil {
		return "", "", err
	}
	return sourcePath, driverPath, nil
}

func command(dir, name string, args ...string) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, -1, fmt.Errorf("%s timed out: %w", name, ctx.Err())
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return stdout.Bytes(), exit.ExitCode(), fmt.Errorf("%s: %s", name, strings.TrimSpace(stderr.String()))
	}
	if err != nil {
		return nil, -1, fmt.Errorf("%s: %w", name, err)
	}
	return stdout.Bytes(), 0, nil
}

func observe(root, sourcePath, driverPath string, count int) ([]observation, error) {
	output, _, err := command(root, "go", "run", sourcePath, driverPath)
	if err != nil {
		return nil, fmt.Errorf("execute generated programs: %w", err)
	}
	var result []observation
	seen := make(map[[2]int]bool)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return nil, fmt.Errorf("invalid observation: %q", scanner.Text())
		}
		index, indexErr := strconv.Atoi(fields[0])
		condition, conditionErr := strconv.ParseBool(fields[1])
		value, valueErr := strconv.Atoi(fields[2])
		if indexErr != nil || conditionErr != nil || valueErr != nil || index < 0 || index >= count || (value != 0 && value != 1) {
			return nil, fmt.Errorf("invalid observation: %q", scanner.Text())
		}
		branchIndex := 0
		if condition {
			branchIndex = 1
		}
		key := [2]int{index, branchIndex}
		if seen[key] {
			return nil, fmt.Errorf("duplicate observation: %q", scanner.Text())
		}
		seen[key] = true
		result = append(result, observation{caseIndex: index, condition: condition, value: value})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(result) != count*2 {
		return nil, fmt.Errorf("got %d observations, want %d", len(result), count*2)
	}
	return result, nil
}

func analyze(root, dir, sourcePath string, all []testCase) (map[string]bool, error) {
	binary := filepath.Join(dir, "messgo")
	if _, _, err := command(root, "go", "build", "-o", binary, "./cmd/messgo"); err != nil {
		return nil, fmt.Errorf("build messgo: %w", err)
	}
	output, code, runErr := command(root, binary, sourcePath, "json", "explicitness")
	if code != 0 && code != 2 {
		return nil, fmt.Errorf("analyze generated programs: exit %d: %w", code, runErr)
	}
	var report jsonReport
	if err := json.Unmarshal(output, &report); err != nil {
		return nil, fmt.Errorf("decode messgo report: %w", err)
	}
	if report.Files == nil || len(report.Errors) != 0 {
		return nil, fmt.Errorf("messgo returned an incomplete report or processing errors: %s", output)
	}
	known := make(map[string]bool, len(all))
	for _, c := range all {
		known[c.name] = true
	}
	outputs := make(map[string]bool)
	violations := 0
	for _, file := range report.Files {
		if filepath.Base(file.File) != filepath.Base(sourcePath) {
			return nil, fmt.Errorf("unexpected report file %q", file.File)
		}
		for _, violation := range file.Violations {
			violations++
			if !known[violation.Function] {
				return nil, fmt.Errorf("unexpected report function %q", violation.Function)
			}
			if violation.Rule != "ImplicitOutput" {
				return nil, fmt.Errorf("unexpected %s finding for %s", violation.Rule, violation.Function)
			}
			outputs[violation.Function] = true
		}
	}
	if (violations == 0 && code != 0) || (violations > 0 && code != 2) {
		return nil, fmt.Errorf("exit %d disagrees with %d violations", code, violations)
	}
	return outputs, nil
}

func verify(all []testCase, observations []observation, outputs map[string]bool) error {
	mutating := make(map[int]bool)
	for _, o := range observations {
		if o.value == 0 {
			continue
		}
		mutating[o.caseIndex] = true
		c := all[o.caseIndex]
		if !outputs[c.name] {
			return fmt.Errorf("caller slice changed from 0 to %d with flag=%t, but %s has no ImplicitOutput:\n%s", o.value, o.condition, c.name, renderCase(c))
		}
	}
	if len(mutating) == 0 {
		return errors.New("no generated case changed the caller slice")
	}
	fmt.Printf("Verified %d functions and %d executions; %d functions changed the caller slice.\n", len(all), len(observations), len(mutating))
	return nil
}

func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return errors.New("run this check from the messgo repository root")
	}
	dir, err := os.MkdirTemp("", "messgo-explicitness-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	all := cases()
	if len(all) != 2016 {
		return fmt.Errorf("generated %d functions, want 2016", len(all))
	}
	source, driver, err := writePrograms(dir, all)
	if err != nil {
		return err
	}
	observations, err := observe(root, source, driver, len(all))
	if err != nil {
		return err
	}
	outputs, err := analyze(root, dir, source, all)
	if err != nil {
		return err
	}
	return verify(all, observations, outputs)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
