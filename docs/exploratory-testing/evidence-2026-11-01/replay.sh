#!/usr/bin/env bash
set -eu
binary=${1:?Pass an absolute path to messgo}
root=${2:?Pass an empty scratch directory}
mkdir -p "$root"
root=$(cd "$root" && pwd)
trap 'jobs -pr | xargs -r kill 2>/dev/null || true' EXIT HUP INT TERM
mkdir -p "$root/module/gen"
cd "$root/module"
cat > go.mod <<'EOF'
module example.com/quality

go 1.26
EOF
cat > main.go <<'EOF'
package quality

func Work() { println("debug") }
EOF
cat > main_test.go <<'EOF'
package quality
func TestDebug() { println("test") }
EOF
cat > os_windows.go <<'EOF'
package quality
func Windows() { println("windows") }
EOF
cat > gen/code.go <<'EOF'
package gen
func Generated() { println("generated") }
EOF
cat > policy.xml <<'EOF'
<ruleset name="team">
  <exclude-pattern>*/gen/*</exclude-pattern>
  <rule ref="go"><exclude name="DevelopmentCodeFragment" /></rule>
</ruleset>
EOF
run() {
  printf '\n$ messgo'; printf ' %q' "$@"; printf '\n'
  set +e
  "$binary" "$@"
  status=$?
  set -e
  printf 'exit=%s\n' "$status"
}
run ./... text go --ignore-tests
run ./... text go
run os_windows.go text go
run ./... text policy.xml --ignore-tests
cat > filter.xml <<'EOF'
<ruleset name="filter">
  <exclude-pattern>*/gen/*</exclude-pattern>
  <rule ref="DevelopmentCodeFragment" />
</ruleset>
EOF
run ./... text filter.xml --ignore-tests
run gen/code.go text filter.xml
run ./... json go --ignore-tests --reportfile reports/result.json
printf '\n$ python3 inspect report\n'
python3 - <<'PY'
import json
with open('reports/result.json') as f: report=json.load(f)
print(json.dumps(report, indent=2))
PY
cat > main.go <<'EOF'
package quality
func Work() {}
EOF
run ./... json go --ignore-tests --reportfile reports/result.json
printf '\n$ python3 inspect overwritten report\n'
python3 - <<'PY'
import json
with open('reports/result.json') as f: report=json.load(f)
print(json.dumps(report, indent=2))
PY
cat > bad.go <<'EOF'
package quality
func Broken( {
EOF
run ./... json go --ignore-tests
run ./... json go --ignore-tests --ignore-errors-on-exit
rm bad.go
run ./... text go --only DevelopmentCodeFragment --ignore-tests
mkdir -p "$root/effects"
cd "$root/effects"
cat > effects.go <<'EOF'
package effects

var global int
func Set(v int) { global = v }
func Get() int { return global }
func Add(a, b int) int { return a + b }
func Alias(p *int) { q := p; *q = 1 }
func Private() { p := new(int); *p = 1 }
func Apply(callback func()) { callback() }
type Counter struct { N int }
func (c *Counter) Inc() { c.N++ }
func (c *Counter) Read() int { return c.N }
EOF
run effects.go text explicitness
run effects.go text explicitness-strict
run effects.go json explicitness --only ImplicitOutput --reportfile result.json
printf '\n$ python3 inspect effect report\n'
python3 - <<'PY'
import json
with open('result.json') as f: report=json.load(f)
for file in report['files']:
    for violation in file['violations']:
        print(violation['function'], violation['rule'], violation['description'])
PY
