import copy
import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch
import collect_test_diagnostics as diagnostics
import report_tests

ROOT = Path(__file__).resolve().parents[1]


class ReportsTest(unittest.TestCase):
    def test_real_dry_run_coverage_and_rejected_mutations(self):
        items = report_tests.specs(ROOT / 'artifacts/reports/coverage.json')
        manifest = json.loads((ROOT / 'hack/ci-shards.json').read_text())
        self.assertEqual(report_tests.validate(items, manifest), 100)
        with self.assertRaises(ValueError):
            report_tests.validate(items[:-1], manifest)
        changed = copy.deepcopy(items)
        changed[0]['LeafNodeLabels'] = ['shard-999']
        with self.assertRaises(ValueError):
            report_tests.validate(changed, manifest)

    def test_summary_excludes_skipped_and_orders_by_duration(self):
        def spec(name, state, duration):
            return dict(LeafNodeText=name, ContainerHierarchyTexts=['group'], State=state, RunTime=duration)
        text = report_tests.summary([spec('fast', 'passed', 1000000000), spec('slow', 'failed', 2000000000), spec('skip', 'skipped', 0)])
        self.assertLess(text.index('slow'), text.index('fast'))
        self.assertNotIn('skip', text)

    def test_diagnostics_timeout_does_not_mask_failure(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(diagnostics.subprocess, 'run', side_effect=subprocess.TimeoutExpired('kubectl', 12)) as run:
            diagnostics.collect(directory)
            self.assertEqual(len(list(Path(directory).glob('*.txt'))), 4)
            for call in run.call_args_list:
                self.assertIn('--context=kind-kubegres', call.args[0])
                self.assertEqual(call.kwargs['timeout'], 12)


if __name__ == '__main__':
    unittest.main()
