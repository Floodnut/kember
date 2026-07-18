import csv
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


class SummarizeTest(unittest.TestCase):
    def test_reports_status_harness_timing_validation(self):
        fields = (
            "mode",
            "iteration",
            "duration_ms",
            "outcome",
            "resource_name",
            "assignment_delta_ms",
            "active_delta_ms",
            "timing_tolerance_ms",
            "timing_consistent",
        )
        rows = [
            {"mode": "raw-job", "iteration": "001", "duration_ms": "4000", "outcome": "Succeeded", "resource_name": "raw"},
            {"mode": "kember-job", "iteration": "001", "duration_ms": "3500", "outcome": "Succeeded", "resource_name": "job", "assignment_delta_ms": "100", "active_delta_ms": "200", "timing_tolerance_ms": "1500", "timing_consistent": "true"},
            {"mode": "warm-lease", "iteration": "001", "duration_ms": "500", "outcome": "Succeeded", "resource_name": "warm", "assignment_delta_ms": "150", "active_delta_ms": "250", "timing_tolerance_ms": "1500", "timing_consistent": "true"},
        ]
        with tempfile.TemporaryDirectory() as directory:
            results = Path(directory) / "results.csv"
            summary = Path(directory) / "summary.json"
            with results.open("w", newline="") as output:
                writer = csv.DictWriter(output, fieldnames=fields)
                writer.writeheader()
                writer.writerows(rows)

            completed = subprocess.run(
                [sys.executable, str(Path(__file__).with_name("summarize.py")), str(results), str(summary)],
                check=False,
                capture_output=True,
                text=True,
            )

            self.assertEqual(completed.returncode, 0, completed.stderr)
            result = json.loads(summary.read_text())
            self.assertEqual(result["timing_validation"]["samples"], 2)
            self.assertEqual(result["timing_validation"]["max_absolute_active_delta_ms"], 250)
            self.assertTrue(result["timing_validation"]["consistent"])


if __name__ == "__main__":
    unittest.main()
