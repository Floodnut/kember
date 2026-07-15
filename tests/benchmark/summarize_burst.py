#!/usr/bin/env python3
import csv
import json
import math
import statistics
import sys
from collections import defaultdict


def percentile(values, fraction):
    ordered = sorted(values)
    return ordered[max(0, math.ceil(len(ordered) * fraction) - 1)]


def main():
    task_path, burst_path, output_path = sys.argv[1:]
    with open(task_path, newline="") as source:
        tasks = list(csv.DictReader(source))
    with open(burst_path, newline="") as source:
        bursts = list(csv.DictReader(source))

    worker_uids = [row["worker_uid"] for row in tasks]
    if len(worker_uids) != len(set(worker_uids)):
        raise SystemExit("a single-use worker UID appeared in more than one measured TaskRun")

    grouped_tasks = defaultdict(list)
    grouped_bursts = defaultdict(list)
    for row in tasks:
        grouped_tasks[(int(row["pool_size"]), int(row["concurrency"]))].append(row)
    for row in bursts:
        grouped_bursts[(int(row["pool_size"]), int(row["concurrency"]))].append(row)

    summary = {"scenarios": []}
    for key in sorted(grouped_bursts):
        pool_size, concurrency = key
        task_rows = grouped_tasks[key]
        burst_rows = grouped_bursts[key]
        queue = [int(row["queue_wait_ms"]) for row in task_rows]
        execution = [int(row["exec_duration_ms"]) for row in task_rows]
        e2e = [int(row["task_e2e_ms"]) for row in task_rows]
        makespans = [int(row["makespan_ms"]) for row in burst_rows]
        throughputs = [float(row["throughput_tasks_per_second"]) for row in burst_rows]
        scenario = {
            "pool_size": pool_size,
            "concurrency": concurrency,
            "repetitions": len(burst_rows),
            "tasks": len(task_rows),
            "failures": sum(row["outcome"] != "Succeeded" for row in task_rows),
            "unique_workers": len({row["worker_uid"] for row in task_rows}),
            "queue_wait_ms": {"p50": percentile(queue, 0.50), "p95": percentile(queue, 0.95)},
            "exec_duration_ms": {"p50": percentile(execution, 0.50), "p95": percentile(execution, 0.95)},
            "task_e2e_ms": {"p50": percentile(e2e, 0.50), "p95": percentile(e2e, 0.95)},
            "burst_makespan_ms": {"median": int(statistics.median(makespans)), "p95": percentile(makespans, 0.95)},
            "throughput_tasks_per_second": {"median": round(statistics.median(throughputs), 3)},
            "observed_max_parallel": max(int(row["observed_max_parallel"]) for row in burst_rows),
        }
        summary["scenarios"].append(scenario)

    with open(output_path, "w") as output:
        json.dump(summary, output, indent=2)
        output.write("\n")

    print("pool concurrency failures makespan_p50_ms throughput_tps queue_p95_ms exec_p95_ms max_parallel")
    for row in summary["scenarios"]:
        print(
            row["pool_size"],
            row["concurrency"],
            row["failures"],
            row["burst_makespan_ms"]["median"],
            row["throughput_tasks_per_second"]["median"],
            row["queue_wait_ms"]["p95"],
            row["exec_duration_ms"]["p95"],
            row["observed_max_parallel"],
        )


if __name__ == "__main__":
    main()
