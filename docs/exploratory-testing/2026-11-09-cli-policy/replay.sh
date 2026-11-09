#!/usr/bin/env bash
set -eu
binary=$(realpath "$1")
work=$(realpath "$2")
mkdir -p "$work"
if [ -n "$(find "$work" -mindepth 1 -maxdepth 1 -print -quit)" ]; then
  printf 'scratch directory must be empty: %s\n' "$work" >&2
  exit 1
fi
cd "$work"
mkdir -p app/generated
printf 'module example.com/explore\n\ngo 1.26\n' > app/go.mod
printf 'package app\nfunc Work() { println("debug") }\n' > app/main.go
printf 'package app\nfunc TestDebug() { println("test") }\n' > app/main_test.go
printf '//go:build windows\n\npackage app\nfunc Windows() { println("windows") }\n' > app/feature_windows.go
printf 'package generated\nfunc Generated() { println("generated") }\n' > app/generated/gen.go
printf '<ruleset name="team"><exclude-pattern>*/generated/*</exclude-pattern><rule ref="go"/></ruleset>\n' > team.xml
run() {
  printf '\n$ messgo'; printf ' %s' "$@"; printf '\n'
  set +e
  "$binary" "$@" 2>&1
  code=$?
  set -e
  printf 'exit=%s\n' "$code"
}
cd app
run ./... text go --ignore-tests --only DevelopmentCodeFragment
run ./... text go --only DevelopmentCodeFragment
run feature_windows.go text go --only DevelopmentCodeFragment
run ./... text ../team.xml --ignore-tests --only DevelopmentCodeFragment
run ./... json go --ignore-tests --only DevelopmentCodeFragment --reportfile reports/result.json
printf '\nSaved JSON findings:\n'
python3 -c 'import json; d=json.load(open("reports/result.json")); print(json.dumps(d,indent=2,sort_keys=True))'
printf 'package app\nfunc Work() {}\n' > main.go
run ./... json go --ignore-tests --only DevelopmentCodeFragment --reportfile reports/result.json
printf '\nReplaced JSON findings:\n'
python3 -c 'import json; d=json.load(open("reports/result.json")); print(json.dumps(d,indent=2,sort_keys=True))'
printf 'package app\nfunc broken(\n' > broken.go
run ./... text go --ignore-tests --only DevelopmentCodeFragment
run ./... text go --ignore-tests --only DevelopmentCodeFragment --ignore-errors-on-exit
rm broken.go
printf '\nCustom threshold and selection:\n'
printf '<ruleset name="team"><rule ref="naming/LongVariable"><properties><property name="maximum" value="8"/></properties></rule></ruleset>\n' > ../threshold.xml
printf 'package app\nfunc Lengthy() { veryLongVariableName := 1; _ = veryLongVariableName }\n' > names.go
run names.go text ../threshold.xml
run names.go text ../threshold.xml --disable LongVariable
