//go:build unix

package githooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrePushAllowsLongerCoverageSetup(t *testing.T) {
	repo, env := newPrePushRepo(t)
	output, err := runPrePush(repo, env)
	if err != nil {
		t.Fatalf("hook rejected coverage setup that needs more than 10 seconds: %v\n%s", err, output)
	}
	if !strings.Contains(output, "pre-push: all checks passed") {
		t.Fatalf("hook did not report success: %s", output)
	}
}

func TestPrePushReportsCoverageFailureWithoutClaimingLowScore(t *testing.T) {
	repo, env := newPrePushRepo(t)
	env = append(env, "FAIL_COVERAGE=1")
	output, err := runPrePush(repo, env)
	if err == nil {
		t.Fatalf("hook accepted a coverage setup failure:\n%s", output)
	}
	if !strings.Contains(output, "coverage collection failed") {
		t.Fatalf("hook hid the coverage failure: %s", output)
	}
	if strings.Contains(output, "covered-MSI below 80%") {
		t.Fatalf("hook attributed a coverage failure to a mutation score: %s", output)
	}
}

func newPrePushRepo(t *testing.T) (string, []string) {
	t.Helper()
	repo, env := newHookRepo(t)
	hook, err := os.ReadFile("pre-push")
	if err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(repo, "githooks", "pre-push"), string(hook))
	writeExecutable(t, filepath.Join(repo, "bin", "mutago"), `#!/bin/sh
timeout=10
while [ "$#" -gt 0 ]; do
    case "$1" in
        --exec-timeout) timeout="$2"; shift ;;
        --exec-timeout=*) timeout="${1#*=}" ;;
    esac
    shift
done
if [ "$FAIL_COVERAGE" = 1 ]; then
    echo "coverage collection failed" >&2
    exit 1
fi
if [ "$timeout" -lt 30 ]; then
    echo "coverage collection timed out" >&2
    exit 1
fi
`)
	runGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	return repo, env
}

func runPrePush(repo string, env []string) (string, error) {
	cmd := exec.Command(filepath.Join(repo, "githooks", "pre-push"))
	cmd.Dir = repo
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return string(output), err
}
