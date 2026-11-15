#!/usr/bin/env python3
import collections
import itertools
import json
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[4]
FILES = (('a.go', 'example', 'A'), ('b_test.go', 'example', 'B'), ('child/c.go', 'child', 'C'))
STATES = tuple(itertools.product((19, 20, 21), (False, True)))


def execute(args, cwd):
    return subprocess.run(args, cwd=cwd, text=True, capture_output=True, timeout=120)


def write_source(path, package, function, state):
    length, writes = state
    name = 'a' * length
    action = 'items[0] = 1' if writes else '_ = items'
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(f'package {package}\nfunc {function}(items []int) {{\n var {name} int\n _ = {name}\n {action}\n}}\n')


def expected(root, states, selected):
    findings = []
    for index in selected:
        file, _, function = FILES[index]
        length, writes = states[index]
        if length > 20:
            findings.append((str(root / file), 'LongVariable', 3, None, 3))
        if writes:
            findings.append((str(root / file), 'ImplicitOutput', 5, function, 3))
    return collections.Counter(findings)


def inspect(binary, root, states, path, options, selected):
    result = execute([str(binary), path, 'json', 'naming,explicitness', '--only', 'LongVariable,ImplicitOutput', *options], root)
    want = expected(root, states, selected)
    code = 2 if want else 0
    if result.returncode != code:
        raise RuntimeError(f'{states} {path} {options}: expected exit {code}, got {result.returncode}; stderr={result.stderr}; stdout={result.stdout}')
    try:
        report = json.loads(result.stdout)
        if not isinstance(report, dict) or report.get('errors') or not isinstance(report.get('files'), list):
            raise ValueError('report has errors or no file list')
        actual = collections.Counter()
        for group in report['files']:
            if not group['violations']:
                raise ValueError('report contains an empty violation group')
            file = str((root / group['file']).resolve())
            for item in group['violations']:
                actual[(file, item['rule'], item['beginLine'], item['function'] if item['rule'] != 'LongVariable' else None, item['priority'])] += 1
    except (ValueError, KeyError, TypeError) as error:
        raise RuntimeError(f'{states} {path} {options}: invalid JSON report: {error}; stdout={result.stdout}') from error
    if actual != want:
        raise RuntimeError(f'{states} {path} {options}: expected {want}, got {actual}')


def main():
    with tempfile.TemporaryDirectory(prefix='messgo-package-results-') as directory:
        root = Path(directory).resolve()
        binary = root / 'messgo'
        build = execute(['go', 'build', '-o', str(binary), './cmd/messgo'], ROOT)
        if build.returncode:
            raise RuntimeError(f'messgo build failed: {build.stderr}')
        module = root / 'module'
        module.mkdir()
        (module / 'go.mod').write_text('module example.test/generated\n\ngo 1.26\n')
        scans = 0
        for states in itertools.product(STATES, repeat=len(FILES)):
            for (file, package, function), state in zip(FILES, states):
                write_source(module / file, package, function, state)
            for path, options, selected in (
                ('.', (), (0, 1, 2)),
                ('./...', (), (0, 1, 2)),
                ('.', ('--ignore-tests',), (0, 2)),
                ('child', (), (2,)),
            ):
                inspect(binary, module, states, path, options, selected)
                scans += 1
        compiled = execute(['go', 'test', './...'], module)
        if compiled.returncode:
            raise RuntimeError(f'generated module failed Go compilation: {compiled.stdout} {compiled.stderr}')
        for length in (34, 35, 36):
            for (file, package, function) in FILES:
                write_source(module / file, package, function, (19, False))
            write_source(module / FILES[0][0], FILES[0][1], FILES[0][2], (length, False))
            result = execute([str(binary), './...', 'json', 'go'], module)
            try:
                report = json.loads(result.stdout)
                if any(not group['violations'] for group in report['files']):
                    raise ValueError('empty violation group')
                actual = collections.Counter(
                    (str((module / group['file']).resolve()), item['rule'], item['beginLine'], item['priority'])
                    for group in report['files'] for item in group['violations']
                )
            except (ValueError, KeyError, TypeError) as error:
                raise RuntimeError(f'default go ruleset: invalid report: {result.stdout}') from error
            want = collections.Counter([(str(module / 'a.go'), 'LongVariable', 3, 3)] if length > 35 else [])
            if report.get('errors') or actual != want or result.returncode != (2 if want else 0):
                raise RuntimeError(f'default go ruleset length {length}: expected {want}, got {actual}; exit {result.returncode}; stderr={result.stderr}')

        (module / 'broken.go').write_text('package example\nfunc Broken( {\n')
        for state in ((19, False), (21, True)):
            states = (state,) * len(FILES)
            for (file, package, function), value in zip(FILES, states):
                write_source(module / file, package, function, value)
            result = execute([str(binary), '.', 'json', 'naming,explicitness', '--only', 'LongVariable,ImplicitOutput'], module)
            try:
                report = json.loads(result.stdout)
                errors = report['errors']
                error_files = [str((module / error['fileName']).resolve()) for error in errors]
                if any(not isinstance(error['message'], str) or not error['message'] for error in errors):
                    raise ValueError('missing parse error message')
                if any(not group['violations'] for group in report['files']):
                    raise ValueError('empty violation group')
                actual = collections.Counter(
                    (str((module / group['file']).resolve()), item['rule'], item['beginLine'], item['function'] if item['rule'] != 'LongVariable' else None, item['priority'])
                    for group in report['files'] for item in group['violations']
                )
            except (ValueError, KeyError, TypeError) as error:
                raise RuntimeError(f'invalid processing-error report: {result.stdout}') from error
            if result.returncode != 3 or error_files != [str(module / 'broken.go')] or actual != expected(module, states, (0, 1, 2)):
                raise RuntimeError(f'parse-error scan: exit={result.returncode}, error files={error_files}, expected findings={expected(module, states, (0, 1, 2))}, actual findings={actual}; stderr={result.stderr}')
        print(f'Checked 216 generated configurations, {scans} selected-rule scans, 3 default-rule scans, and 2 parse-error scans.')


if __name__ == '__main__':
    main()
