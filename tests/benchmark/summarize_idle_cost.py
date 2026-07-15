#!/usr/bin/env python3
import argparse
import csv
import json
import math
import pathlib
import statistics


def percentile(values, fraction):
    ordered = sorted(values)
    if not ordered:
        return 0.0
    index = (len(ordered) - 1) * fraction
    lower = math.floor(index)
    upper = math.ceil(index)
    if lower == upper:
        return ordered[lower]
    return ordered[lower] * (upper - index) + ordered[upper] * (index - lower)


def value(metric, key):
    raw = metric.get(key, {}).get("value", 0)
    return float(raw or 0)


def summarize(values):
    return {
        "mean": statistics.fmean(values) if values else 0.0,
        "p50": percentile(values, 0.50),
        "p95": percentile(values, 0.95),
    }


parser = argparse.ArgumentParser()
parser.add_argument("output_dir", type=pathlib.Path)
args = parser.parse_args()

rows = []
summary = {}
for size in (1, 4):
    pod_list = json.loads((args.output_dir / f"pods-size{size}.json").read_text())
    active_pods = [
        item
        for item in pod_list["items"]
        if not item["metadata"].get("deletionTimestamp")
        and item.get("status", {}).get("phase") == "Running"
        and any(
            condition.get("type") == "Ready" and condition.get("status") == "True"
            for condition in item.get("status", {}).get("conditions", [])
        )
    ]
    pod_names = {item["metadata"]["name"] for item in active_pods}
    if len(pod_names) != size:
        raise SystemExit(
            f"size {size}: expected {size} active worker Pods in snapshot, got {sorted(pod_names)}"
        )
    totals = {"cpu_millicores": [], "working_set_mib": [], "rss_mib": []}
    worker_sample_counts = []

    for sample_path in sorted(args.output_dir.glob(f"cri-size{size}-*.json")):
        stats = json.loads(sample_path.read_text()).get("stats", [])
        sample_number = int(sample_path.stem.rsplit("-", 1)[1])
        selected = []
        for stat in stats:
            labels = stat.get("attributes", {}).get("labels", {})
            if labels.get("io.kubernetes.pod.namespace") != active_pods[0]["metadata"]["namespace"]:
                continue
            if labels.get("io.kubernetes.pod.name") not in pod_names:
                continue
            selected.append(stat)

        worker_sample_counts.append(len(selected))
        sample_totals = {key: 0.0 for key in totals}
        for stat in selected:
            labels = stat["attributes"]["labels"]
            cpu = value(stat.get("cpu", {}), "usageNanoCores") / 1_000_000
            working_set = value(stat.get("memory", {}), "workingSetBytes") / 1_048_576
            rss = value(stat.get("memory", {}), "rssBytes") / 1_048_576
            row = {
                "pool_size": size,
                "sample": sample_number,
                "pod_name": labels["io.kubernetes.pod.name"],
                "pod_uid": labels.get("io.kubernetes.pod.uid", ""),
                "cpu_millicores": cpu,
                "working_set_mib": working_set,
                "rss_mib": rss,
            }
            rows.append(row)
            sample_totals["cpu_millicores"] += cpu
            sample_totals["working_set_mib"] += working_set
            sample_totals["rss_mib"] += rss
        for key, total in sample_totals.items():
            totals[key].append(total)

    if not worker_sample_counts or any(count != size for count in worker_sample_counts):
        raise SystemExit(
            f"size {size}: expected {size} worker containers in every sample, got {worker_sample_counts}"
        )
    summary[str(size)] = {
        "samples": len(worker_sample_counts),
        "workers_per_sample": size,
        "pool_total": {key: summarize(values) for key, values in totals.items()},
        "per_worker_mean": {
            key: statistics.fmean(values) / size for key, values in totals.items()
        },
    }

with (args.output_dir / "worker-samples.csv").open("w", newline="") as output:
    writer = csv.DictWriter(output, fieldnames=rows[0].keys())
    writer.writeheader()
    writer.writerows(rows)
(args.output_dir / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
print(json.dumps(summary, indent=2))
