#!/usr/bin/env python3
import os
import signal
import subprocess
import sys

process = subprocess.Popen(
    ['go', 'run', './.agents/skills/verify-ruleset-transitions/scripts/check.go', *sys.argv[1:]],
    start_new_session=True,
)
try:
    sys.exit(process.wait(timeout=120))
except subprocess.TimeoutExpired:
    os.killpg(process.pid, signal.SIGKILL)
    process.wait()
    sys.exit('ruleset check exceeded 120 seconds')
