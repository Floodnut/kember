#!/usr/bin/env python3
import csv
import json
import math
import os
import statistics
import sys
from collections import defaultdict
from pathlib import Path


def percentile(values, percent):
    ordered = sorted(values)
    return ordered[max(0, math.ceil(len(ordered) * percent / 100) - 1)]


if len(sys.argv) != 4:
    raise SystemExit("usage: summarize_arrival.py TASKS.csv CONDITIONS.csv SUMMARY.json")

task_path, condition_path, summary_path = map(Path, sys.argv[1:])
tasks = defaultdict(lambda: defaultdict(list))
failures = defaultdict(int)
worker_uids = []
with task_path.open(newline="") as source:
    for row in csv.DictReader(source):
        key = (int(row["pool_size"]), float(row["arrival_interval_seconds"]))
        worker_uids.append(row["worker_uid"])
        if row["outcome"] != "Succeeded":
            failures[key] += 1
            continue
        for metric in ("queue_wait_ms", "exec_duration_ms", "task_e2e_ms"):
            tasks[key][metric].append(int(row[metric]))

if len(worker_uids) != len(set(worker_uids)):
    raise SystemExit("a single-use worker UID appeared in more than one measured TaskRun")

conditions = defaultdict(lambda: defaultdict(list))
with condition_path.open(newline="") as source:
    for row in csv.DictReader(source):
        key = (int(row["pool_size"]), float(row["arrival_interval_seconds"]))
        for metric in (
            "makespan_ms", "throughput_tasks_per_second", "observed_max_parallel",
            "reserved_worker_seconds", "reserved_cpu_core_seconds", "reserved_memory_mib_seconds",
        ):
            conditions[key][metric].append(float(row[metric]))

summary = {"conditions": {}}
for key in sorted(tasks, key=lambda item: (item[1], item[0])):
    size, interval = key
    label = f"interval_{interval:g}_capacity_{size}"
    entry = {"tasks": len(tasks[key]["task_e2e_ms"]), "failures": failures[key]}
    for metric, values in tasks[key].items():
        entry[metric] = {
            "p50": percentile(values, 50),
            "p95": percentile(values, 95),
            "mean": round(statistics.mean(values), 1),
        }
    for metric, values in conditions[key].items():
        entry[metric] = {
            "mean": round(statistics.mean(values), 3),
            "p95": round(percentile(values, 95), 3),
        }
    summary["conditions"][label] = entry

def condition(interval, size):
    return summary["conditions"][f"interval_{interval:g}_capacity_{size}"]

all_succeeded = all(entry["failures"] == 0 for entry in summary["conditions"].values())
slow_interval = float(os.environ.get("SLOW_INTERVAL", "5"))
has_burst = all(f"interval_0_capacity_{size}" in summary["conditions"] for size in (1, 4))
has_slow = all(
    f"interval_{slow_interval:g}_capacity_{size}" in summary["conditions"] for size in (1, 4)
)
burst_queue_reduction = None
slow_e2e_reduction = None
slow_reservation_ratio = None
if has_burst:
    burst_queue_reduction = 1 - (
        condition(0, 4)["queue_wait_ms"]["p95"] / condition(0, 1)["queue_wait_ms"]["p95"]
    )
if has_slow:
    slow_e2e_reduction = 1 - (
        condition(slow_interval, 4)["task_e2e_ms"]["p95"]
        / condition(slow_interval, 1)["task_e2e_ms"]["p95"]
    )
    slow_reservation_ratio = (
        condition(slow_interval, 4)["reserved_worker_seconds"]["mean"]
        / condition(slow_interval, 1)["reserved_worker_seconds"]["mean"]
    )
summary["verdict"] = {
    "all_succeeded": all_succeeded,
    "slow_interval_seconds": slow_interval,
    "burst_queue_reduction_percent": round(burst_queue_reduction * 100, 1) if has_burst else None,
    "slow_arrival_e2e_reduction_percent": round(slow_e2e_reduction * 100, 1) if has_slow else None,
    "slow_arrival_reservation_ratio": round(slow_reservation_ratio, 2) if has_slow else None,
    "burst_gate_passed": burst_queue_reduction >= 0.50 if has_burst else None,
    "slow_arrival_no_extra_benefit": slow_e2e_reduction < 0.10 if has_slow else None,
    "reservation_accounting_passed": 3.5 <= slow_reservation_ratio <= 4.5 if has_slow else None,
}
summary_path.write_text(json.dumps(summary, indent=2) + "\n")
print(json.dumps(summary, indent=2))
