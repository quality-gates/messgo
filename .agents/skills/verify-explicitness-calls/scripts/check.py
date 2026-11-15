#!/usr/bin/env python3
import itertools
import json
import pathlib
import subprocess
import tempfile

root = pathlib.Path(__file__).resolve().parents[4]


def run(args, accepted=(0,)):
    result = subprocess.run(args, cwd=root, text=True, capture_output=True, timeout=120)
    if result.returncode not in accepted:
        raise RuntimeError(f'{args[0]} exited {result.returncode}: {result.stderr}')
    return result.stdout


def programs():
    source = ['package main\n']
    driver = ['package main\nimport "fmt"\nfunc main() {\n']
    names = []
    for index, (depth, initial, mode, branch, flow) in enumerate(itertools.product(range(1, 4), ('p', '[]int{0}'), ('chain', 'recursive'), ('always', 'conditional'), ('direct', 'loop'))):
        name = f'Case{index:03}'
        names.append(name)
        helper = f'helper{index}'
        if mode == 'recursive':
            source.append(f'func {helper}(p []int, n int) {{ if n > 0 {{ {helper}(p, n-1) }} else {{ p[0] = 1 }} }}\n')
        else:
            source.append(f'func {helper}(p []int, n int) {{ p[0] = 1 }}\n')
        for layer in range(depth):
            prior = helper if layer == 0 else f'{helper}_{layer-1}'
            source.append(f'func {helper}_{layer}(p []int, n int) {{ {prior}(p,n) }}\n')
        call = f'{helper}_{depth-1}(q, 2)'
        if branch == 'conditional':
            call = f'if flag {{ {call} }}'
        if flow == 'loop':
            call = f'for i := 0; i < 2; i++ {{ {call} }}'
        source.append(f'func {name}(p []int, flag bool) {{ q := {initial}; {call} }}\n')
        driver.append(f'for _, flag := range []bool{{false,true}} {{ p := []int{{0}}; {name}(p,flag); fmt.Printf("{name} %t %d\\n", flag, p[0]) }}\n')
    driver.append('}\n')
    return ''.join(source), ''.join(driver), names


def main():
    source, driver, names = programs()
    with tempfile.TemporaryDirectory(prefix='messgo-call-check-') as directory:
        temp = pathlib.Path(directory)
        casefile, driverfile, binary = temp / 'cases.go', temp / 'driver.go', temp / 'messgo'
        casefile.write_text(source)
        driverfile.write_text(driver)
        observations = run(['go', 'run', str(casefile), str(driverfile)]).splitlines()
        parsed = {}
        for observation in observations:
            fields = observation.split()
            if len(fields) != 3:
                raise RuntimeError(f'invalid observation: {observation}')
            name, flag, value = fields
            key = (name, flag)
            if name not in names or flag not in ('true', 'false') or value not in ('0', '1') or key in parsed:
                raise RuntimeError(f'invalid or duplicated observation: {observation}')
            parsed[key] = value
        if set(parsed) != set(itertools.product(names, ('false', 'true'))):
            raise RuntimeError('incomplete runtime observations')
        run(['go', 'build', '-o', str(binary), './cmd/messgo'])
        output = run([str(binary), str(casefile), 'json', 'explicitness'], (0, 2))
        report = json.loads(output)
        if report.get('errors') or len(report.get('files', [])) != 1:
            raise RuntimeError(f'incomplete report: {output}')
        findings = {v['function'] for v in report['files'][0]['violations'] if v['rule'] == 'ImplicitOutput'}
        errors = []
        for (name, flag), value in parsed.items():
            if value == '1' and name not in findings:
                errors.append(f'{name} flag={flag} changed the caller slice without ImplicitOutput')
        if errors:
            raise RuntimeError('\n'.join(errors))
        print(f'Checked {len(names)} generated call graphs and {len(observations)} executions; no caller mutation missed.')


if __name__ == '__main__':
    main()
