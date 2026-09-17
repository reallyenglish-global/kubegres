#!/usr/bin/env python3
"""Bounded, read-only diagnostics for the disposable Kind test cluster only."""
import subprocess
import sys
from pathlib import Path


def collect(destination):
    dest = Path(destination)
    dest.mkdir(parents=True, exist_ok=True)
    prefix = ['kubectl', '--context=kind-kubegres', '--request-timeout=10s']
    commands = {
        'pods.txt': ['get', 'pods', '-A', '-o', 'wide'],
        'events.txt': ['get', 'events', '-A', '--sort-by=.lastTimestamp'],
        'statefulsets.txt': ['get', 'statefulsets', '-A', '-o', 'wide'],
        'pvcs.txt': ['get', 'pvc', '-A'],
    }
    for name, args in commands.items():
        try:
            result = subprocess.run(prefix + args, capture_output=True, text=True, timeout=12, check=False)
            (dest / name).write_text(result.stdout + result.stderr)
        except (subprocess.TimeoutExpired, OSError) as exc:
            (dest / name).write_text(str(exc))


if __name__ == '__main__':
    collect(sys.argv[1])
