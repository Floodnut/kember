import unittest

from lifecycle_timing import compare_taskrun_timing


class LifecycleTimingTest(unittest.TestCase):
    def test_compares_assignment_and_active_intervals(self):
        taskrun = {
            "metadata": {"creationTimestamp": "2026-07-18T01:00:01Z"},
            "status": {
                "dispatchedAt": "2026-07-18T01:00:03Z",
                "completedAt": "2026-07-18T01:00:08Z",
            },
        }

        timing = compare_taskrun_timing(taskrun, 1000, 3200, 8300, 500)

        self.assertEqual(timing["status_assignment_ms"], 2000)
        self.assertEqual(timing["status_active_ms"], 5000)
        self.assertEqual(timing["assignment_delta_ms"], 200)
        self.assertEqual(timing["active_delta_ms"], 100)
        self.assertTrue(timing["timing_consistent"])

    def test_rejects_interval_outside_tolerance(self):
        taskrun = {
            "metadata": {"creationTimestamp": "2026-07-18T01:00:01Z"},
            "status": {
                "dispatchedAt": "2026-07-18T01:00:03Z",
                "completedAt": "2026-07-18T01:00:08Z",
            },
        }

        timing = compare_taskrun_timing(taskrun, 1000, 5000, 10000, 500)

        self.assertFalse(timing["timing_consistent"])


if __name__ == "__main__":
    unittest.main()
