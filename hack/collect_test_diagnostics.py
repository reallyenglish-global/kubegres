#!/usr/bin/env python3
"""Bounded, read-only diagnostics for the disposable Kind test cluster only."""
import argparse
import subprocess
from pathlib import Path


def collect(destination, kubeconfig, *, extended=False):
    kubeconfig = Path(kubeconfig).resolve(strict=True)
    if not kubeconfig.is_file():
        raise ValueError('Expected an explicit Kind kubeconfig file')
    dest = Path(destination)
    dest.mkdir(parents=True, exist_ok=True)
    prefix = ['kubectl', f'--kubeconfig={kubeconfig}', '--context=kind-kubegres', '--request-timeout=10s']
    commands = {
        'pods.txt': ['get', 'pods', '-A', '-o', 'wide'],
        'events.txt': ['get', 'events', '-A', '--sort-by=.lastTimestamp'],
        'statefulsets.txt': ['get', 'statefulsets', '-A', '-o', 'wide'],
        'pvcs.txt': ['get', 'pvc', '-A'],
    }
    if extended:
        commands.update({
            'kubegres.txt': ['get', 'kubegres', '-A'],
            'pod-descriptions.txt': ['describe', 'pods', '-A'],
            'statefulset-descriptions.txt': ['describe', 'statefulsets', '-A'],
        })
    for name, args in commands.items():
        try:
            result = subprocess.run(prefix + args, capture_output=True, text=True, timeout=12, check=False)
            (dest / name).write_text(result.stdout + result.stderr)
        except (subprocess.TimeoutExpired, OSError) as exc:
            (dest / name).write_text(str(exc))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('destination')
    parser.add_argument('kubeconfig')
    parser.add_argument('--extended', action='store_true')
    collect(**vars(parser.parse_args()))
