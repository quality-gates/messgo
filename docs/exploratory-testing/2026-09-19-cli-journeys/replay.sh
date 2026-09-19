#!/usr/bin/env bash
# Replays confirmed exploratory findings from a clean scratch dir.
# Usage: replay.sh <messgo-binary>
set -u
BIN=${1:?messgo binary}
W=$(mktemp -d)
cd "$W"
printf 'module example.com/r\n\ngo 1.26\n' > go.mod
mkdir -p pkg gen testdata
cat > pkg/a.go <<'GO'
package pkg

// Pair returns two values.
func Pair() (int, int) {
	firstExtremelyLongVariableNameForTestingPurposes, secondExtremelyLongVariableNameForTestingPurposes := 1, 2
	return firstExtremelyLongVariableNameForTestingPurposes, secondExtremelyLongVariableNameForTestingPurposes
}
GO
printf 'package gen\n\nfunc Generated() { println("x") }\n' > gen/gen.go
printf 'package fixture\n\nfunc F() { println(1) }\n' > testdata/fixture.go

echo "== B1 --reportfile into missing directory (README example)"
"$BIN" ./pkg/... sarif go --reportfile reports/messgo.sarif; echo "exit=$?"; ls reports 2>&1

echo "== B2 phpmd-style rulesets/<name>.xml refs"
printf '<ruleset name="r">\n  <rule ref="rulesets/codesize.xml"/>\n</ruleset>\n' > whole.xml
"$BIN" ./pkg/... text whole.xml; echo "exit=$?"
printf '<ruleset name="r">\n  <rule ref="rulesets/naming.xml/LongVariable"/>\n</ruleset>\n' > single.xml
"$BIN" ./pkg/... text single.xml; echo "exit=$?"

echo "== B3 <exclude-pattern> in ruleset XML"
printf '<ruleset name="r">\n  <exclude-pattern>*/gen/*</exclude-pattern>\n  <rule ref="design/DevelopmentCodeFragment"/>\n</ruleset>\n' > excl.xml
"$BIN" ./gen/... text excl.xml -v; echo "exit=$?"

echo "== B4 GitLab fingerprints collide"
"$BIN" ./pkg/... gitlab go | jq -c '[.[].fingerprint] | {total: length, unique: (unique|length)}'

echo "== B5 ./... walks testdata/"
go list ./... 2>&1
"$BIN" ./... text go --only DevelopmentCodeFragment --exclude gen; echo "exit=$?"

rm -rf "$W"
