#!/usr/bin/env python3
"""Fit MLS optimization benchmark CSV rows against log and linear models."""

from __future__ import annotations

import argparse
import csv
import json
import math
from collections import defaultdict
from pathlib import Path


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("csv_path", type=Path)
    parser.add_argument("--metric", default="median_ms", choices=["median_ms", "p95_ms", "p99_ms"])
    parser.add_argument("--json-output", type=Path)
    args = parser.parse_args()

    rows = read_rows(args.csv_path)
    summary = {}
    for operation, op_rows in sorted(rows.items()):
        xs = [float(row["n"]) for row in op_rows]
        ys = [float(row[args.metric]) for row in op_rows]
        log_xs = [math.log2(x) for x in xs]
        log_fit = fit(log_xs, ys)
        linear_fit = fit(xs, ys)
        summary[operation] = {
            "samples": len(op_rows),
            "metric": args.metric,
            "log2_model": {"slope": log_fit[0], "intercept": log_fit[1], "r2": log_fit[2]},
            "linear_model": {"slope": linear_fit[0], "intercept": linear_fit[1], "r2": linear_fit[2]},
            "better_fit": "log2" if log_fit[2] >= linear_fit[2] else "linear",
        }

    print(json.dumps(summary, indent=2, sort_keys=True))
    if args.json_output:
        args.json_output.write_text(json.dumps(summary, indent=2, sort_keys=True), encoding="utf-8")


def read_rows(path: Path) -> dict[str, list[dict[str, str]]]:
    grouped: dict[str, list[dict[str, str]]] = defaultdict(list)
    with path.open(newline="", encoding="utf-8") as f:
        for row in csv.DictReader(f):
            grouped[row["operation"]].append(row)
    for op_rows in grouped.values():
        op_rows.sort(key=lambda row: int(row["n"]))
    return grouped


def fit(xs: list[float], ys: list[float]) -> tuple[float, float, float]:
    if len(xs) != len(ys) or not xs:
        return 0.0, 0.0, 0.0
    x_mean = sum(xs) / len(xs)
    y_mean = sum(ys) / len(ys)
    denom = sum((x - x_mean) ** 2 for x in xs)
    slope = 0.0 if denom == 0 else sum((x - x_mean) * (y - y_mean) for x, y in zip(xs, ys)) / denom
    intercept = y_mean - slope * x_mean
    predicted = [slope * x + intercept for x in xs]
    ss_res = sum((y - y_hat) ** 2 for y, y_hat in zip(ys, predicted))
    ss_tot = sum((y - y_mean) ** 2 for y in ys)
    r2 = 1.0 if ss_tot == 0 else 1.0 - (ss_res / ss_tot)
    return slope, intercept, r2


if __name__ == "__main__":
    main()
