#!/usr/bin/env python3
import csv
import json
import math
import statistics
import sys
from collections import defaultdict
from pathlib import Path


def percentile(values: list[int], percent: int) -> int:
    ordered = sorted(values)
    return ordered[max(0, math.ceil(len(ordered) * percent / 100) - 1)]


def main() -> int:
    if len(sys.argv) != 3:
        print("usage: summarize.py RESULTS.csv SUMMARY.json", file=sys.stderr)
        return 2

    results_path = Path(sys.argv[1])
    summary_path = Path(sys.argv[2])
    durations: dict[str, list[int]] = defaultdict(list)
    failures: dict[str, int] = defaultdict(int)
    by_iteration: dict[str, dict[str, int]] = defaultdict(dict)
    timing_rows: list[dict[str, str]] = []

    with results_path.open(newline="") as source:
        for row in csv.DictReader(source):
            mode = row["mode"]
            if row["outcome"] == "Succeeded":
                duration = int(row["duration_ms"])
                durations[mode].append(duration)
                by_iteration[row["iteration"]][mode] = duration
            else:
                failures[mode] += 1
            if row.get("timing_consistent"):
                timing_rows.append(row)

    modes = ("raw-job", "kember-job", "warm-lease")
    if any(not durations[mode] for mode in modes):
        print("each mode needs at least one successful sample", file=sys.stderr)
        return 1

    summary: dict[str, object] = {"modes": {}}
    for mode in modes:
        values = durations[mode]
        summary["modes"][mode] = {
            "count": len(values),
            "failures": failures[mode],
            "min_ms": min(values),
            "p50_ms": percentile(values, 50),
            "p95_ms": percentile(values, 95),
            "max_ms": max(values),
            "mean_ms": round(statistics.mean(values), 1),
            "stdev_ms": round(statistics.stdev(values), 1) if len(values) > 1 else 0.0,
        }

    raw_p95 = summary["modes"]["raw-job"]["p95_ms"]
    kember_p95 = summary["modes"]["kember-job"]["p95_ms"]
    warm_p95 = summary["modes"]["warm-lease"]["p95_ms"]
    absolute_improvement = raw_p95 - warm_p95
    percent_improvement = absolute_improvement / raw_p95 * 100
    kember_overhead = (kember_p95 - raw_p95) / raw_p95 * 100
    paired_improvements = [
        samples["raw-job"] - samples["warm-lease"]
        for samples in by_iteration.values()
        if "raw-job" in samples and "warm-lease" in samples
    ]

    if any(failures.values()):
        verdict = "Invalid"
    elif percent_improvement >= 30 and absolute_improvement >= 1000:
        verdict = "Strong"
    elif percent_improvement >= 15 and absolute_improvement >= 500:
        verdict = "Conditional"
    else:
        verdict = "Weak"

    summary["comparison"] = {
        "raw_job_p95_ms": raw_p95,
        "warm_lease_p95_ms": warm_p95,
        "absolute_improvement_ms": absolute_improvement,
        "percent_improvement": round(percent_improvement, 1),
        "kember_job_overhead_percent": round(kember_overhead, 1),
        "paired_improvement_count": len(paired_improvements),
        "paired_improvement_min_ms": min(paired_improvements),
        "paired_improvement_p50_ms": percentile(paired_improvements, 50),
        "paired_improvement_max_ms": max(paired_improvements),
        "paired_improvement_mean_ms": round(statistics.mean(paired_improvements), 1),
        "verdict": verdict,
    }
    if timing_rows:
        assignment_deltas = [abs(int(row["assignment_delta_ms"])) for row in timing_rows]
        active_deltas = [abs(int(row["active_delta_ms"])) for row in timing_rows]
        timing_consistent = all(row["timing_consistent"] == "true" for row in timing_rows)
        summary["timing_validation"] = {
            "samples": len(timing_rows),
            "tolerance_ms": max(int(row["timing_tolerance_ms"]) for row in timing_rows),
            "max_absolute_assignment_delta_ms": max(assignment_deltas),
            "max_absolute_active_delta_ms": max(active_deltas),
            "consistent": timing_consistent,
        }
        if not timing_consistent:
            summary["comparison"]["verdict"] = "Invalid"
    verdict = summary["comparison"]["verdict"]
    summary_path.write_text(json.dumps(summary, indent=2) + "\n")

    print("mode,count,failures,min_ms,p50_ms,p95_ms,max_ms,mean_ms,stdev_ms")
    for mode in modes:
        row = summary["modes"][mode]
        print(
            f"{mode},{row['count']},{row['failures']},{row['min_ms']},"
            f"{row['p50_ms']},{row['p95_ms']},{row['max_ms']},{row['mean_ms']},{row['stdev_ms']}"
        )
    print(
        f"verdict={verdict} improvement={absolute_improvement}ms "
        f"({percent_improvement:.1f}%) kember_job_overhead={kember_overhead:.1f}%"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
