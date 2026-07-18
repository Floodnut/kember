#!/usr/bin/env python3
import datetime
import json
import sys


def timestamp_ms(value: str) -> int:
    return int(datetime.datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp() * 1000)


def compare_taskrun_timing(
    taskrun: dict,
    harness_start_ms: int,
    observed_dispatch_ms: int,
    observed_terminal_ms: int,
    tolerance_ms: int,
) -> dict:
    metadata = taskrun["metadata"]
    status = taskrun["status"]
    created_ms = timestamp_ms(metadata["creationTimestamp"])
    dispatched_ms = timestamp_ms(status["dispatchedAt"])
    completed_ms = timestamp_ms(status["completedAt"])

    status_assignment_ms = dispatched_ms - created_ms
    status_active_ms = completed_ms - dispatched_ms
    harness_assignment_ms = observed_dispatch_ms - harness_start_ms
    harness_active_ms = observed_terminal_ms - observed_dispatch_ms
    assignment_delta_ms = harness_assignment_ms - status_assignment_ms
    active_delta_ms = harness_active_ms - status_active_ms
    consistent = abs(assignment_delta_ms) <= tolerance_ms and abs(active_delta_ms) <= tolerance_ms
    return {
        "harness_assignment_ms": harness_assignment_ms,
        "harness_active_ms": harness_active_ms,
        "status_assignment_ms": status_assignment_ms,
        "status_active_ms": status_active_ms,
        "assignment_delta_ms": assignment_delta_ms,
        "active_delta_ms": active_delta_ms,
        "timing_tolerance_ms": tolerance_ms,
        "timing_consistent": consistent,
    }


def main() -> int:
    if len(sys.argv) != 5:
        print(
            "usage: lifecycle_timing.py HARNESS_START_MS OBSERVED_DISPATCH_MS "
            "OBSERVED_TERMINAL_MS TOLERANCE_MS",
            file=sys.stderr,
        )
        return 2
    taskrun = json.load(sys.stdin)
    timing = compare_taskrun_timing(taskrun, *(int(value) for value in sys.argv[1:]))
    fields = (
        "harness_assignment_ms",
        "harness_active_ms",
        "status_assignment_ms",
        "status_active_ms",
        "assignment_delta_ms",
        "active_delta_ms",
        "timing_tolerance_ms",
    )
    print(",".join(str(timing[field]) for field in fields) + "," + str(timing["timing_consistent"]).lower())
    if not timing["timing_consistent"]:
        print(f"TaskRun status and harness timing differ beyond tolerance: {timing}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
