#!/usr/bin/env bash
# Replays every confirmed finding of the 2026-10-10 pass from an empty temp dir.
# Usage: replay.sh /abs/path/to/messgo [python-with-jsonschema] [sarif-schema.json]
# The SARIF check (bug 3) needs a Python with `jsonschema` and the OASIS
# sarif-schema-2.1.0.json; it is skipped when either is missing.
set -u
BIN=$1
PY=${2:-}
SCHEMA=${3:-}
HERE=$(cd "$(dirname "$0")" && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
cd "$WORK"
printf 'module example.com/replay\ngo 1.22\n' > go.mod

echo "### Bug 1: naming length rules count UTF-8 bytes, not characters"
cat > names.go <<'GO'
package replay

func Rechnen(überÄnderungsgrößen int) int {
	名前 := 2
	ab := 1
	return 名前 + ab + überÄnderungsgrößen
}
GO
python3 -c "for s in ['überÄnderungsgrößen','名前','ab']: print(s, 'chars', len(s), 'bytes', len(s.encode()))"
"$BIN" names.go text naming; echo "exit=$?"
rm names.go

echo
echo "### Bug 2: class-level naming rules leave the class field empty"
cat > cls.go <<'GO'
package replay

type Fo struct{ v int }

type bad_name struct{ w int }
GO
cat > cls.xml <<'XML'
<ruleset name="cls">
  <rule ref="rulesets/naming.xml/ShortClassName"/>
  <rule ref="rulesets/naming.xml/LongClassName"><properties><property name="maximum" value="2"/></properties></rule>
  <rule ref="rulesets/controversial.xml/CamelCaseClassName"/>
  <rule ref="rulesets/codesize.xml/TooManyFields"><properties><property name="maxfields" value="0"/></properties></rule>
</ruleset>
XML
"$BIN" cls.go json cls.xml | jq -c '.files[].violations[] | {rule, beginLine, class}'
echo "xml violations / with class attribute:"
"$BIN" cls.go xml cls.xml | grep -c '<violation '
"$BIN" cls.go xml cls.xml | grep -c ' class="'
rm cls.go cls.xml

echo
echo "### Bug 3: controversial rules emit helpUri \"#\" which is not a SARIF uri"
cat > camel.go <<'GO'
package replay

type bad_name struct{ w int }
GO
"$BIN" camel.go sarif controversial > out.sarif; echo "exit=$?"
jq -c '.runs[0].tool.driver.rules[] | {id, helpUri}' out.sarif
if [ -n "$PY" ] && [ -n "$SCHEMA" ]; then
  "$PY" -I "$HERE/validate_sarif.py" "$SCHEMA" out.sarif
else
  echo "(schema validation skipped)"
fi
