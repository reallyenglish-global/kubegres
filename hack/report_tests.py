#!/usr/bin/env python3
"""Summarise real Ginkgo timings; verify each spec belongs to exactly one CI shard."""
import collections
import json
import sys
from pathlib import Path


def specs(path):
    return [s for suite in json.loads(Path(path).read_text()) for s in suite['SpecReports'] if s['LeafNodeType'] == 'It']


def validate(items, manifest):
    expected = {g['file']: g for g in manifest['groups']}
    counts = collections.Counter()
    for s in items:
        filename = Path(s['LeafNodeLocation']['FileName']).name
        labels = s.get('LeafNodeLabels', []) + [x for group in s.get('ContainerHierarchyLabels', []) for x in group]
        shard = [x for x in labels if x.startswith('shard-')]
        if filename not in expected or shard != [expected[filename]['shard']]:
            raise ValueError(f'Wrong/missing/duplicate shard: {filename}: {shard}')
        counts[filename] += 1
    if dict(counts) != {name: g['specs'] for name, g in expected.items()}:
        raise ValueError(f'Incomplete spec coverage: {dict(counts)}')
    return sum(counts.values())


def summary(items):
    rows = []
    for s in items:
        if s['State'] in ('skipped', 'pending'):
            continue
        name = ' / '.join(s['ContainerHierarchyTexts'] + [s['LeafNodeText']])
        rows.append((s['RunTime'] / 1e9, s['State'], name))
    return '\n'.join(['## Spec durations (slowest first)', '', *[f'- {seconds:.1f}s — {state} — {name}' for seconds, state, name in sorted(rows, reverse=True)]]) + '\n'


if __name__ == '__main__':
    items = specs(sys.argv[1])
    if len(sys.argv) > 2:
        print(f'Validated {validate(items, json.loads(Path(sys.argv[2]).read_text()))} specs, exactly one shard each')
    else:
        print(summary(items))
