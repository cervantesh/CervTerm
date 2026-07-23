"""Capture interleaved Windows GUI startup/RSS/idle CPU comparisons.

The harness intentionally force-terminates both candidates after measurement so
shutdown behavior cannot bias startup or steady-state metrics. Run the normal
clean-shutdown GUI/package gates separately.
"""

from __future__ import annotations

import argparse
import ctypes
import hashlib
import json
import os
import platform
import statistics
import subprocess
import time
from ctypes import wintypes
from pathlib import Path

PROCESS_QUERY_INFORMATION = 0x0400
PROCESS_VM_READ = 0x0010


class FileTime(ctypes.Structure):
    _fields_ = [("low", wintypes.DWORD), ("high", wintypes.DWORD)]


class ProcessMemoryCounters(ctypes.Structure):
    _fields_ = [
        ("cb", wintypes.DWORD),
        ("page_fault_count", wintypes.DWORD),
        ("peak_working_set_size", ctypes.c_size_t),
        ("working_set_size", ctypes.c_size_t),
        ("quota_peak_paged_pool_usage", ctypes.c_size_t),
        ("quota_paged_pool_usage", ctypes.c_size_t),
        ("quota_peak_non_paged_pool_usage", ctypes.c_size_t),
        ("quota_non_paged_pool_usage", ctypes.c_size_t),
        ("pagefile_usage", ctypes.c_size_t),
        ("peak_pagefile_usage", ctypes.c_size_t),
    ]


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def visible_window_for_pid(user32: ctypes.WinDLL, pid: int) -> int | None:
    found: list[int] = []
    callback_type = ctypes.WINFUNCTYPE(wintypes.BOOL, wintypes.HWND, wintypes.LPARAM)

    def callback(hwnd: int, _param: int) -> bool:
        owner = wintypes.DWORD()
        user32.GetWindowThreadProcessId(hwnd, ctypes.byref(owner))
        if owner.value == pid and user32.IsWindowVisible(hwnd):
            found.append(hwnd)
        return True

    user32.EnumWindows(callback_type(callback), 0)
    return found[0] if found else None


def process_cpu_seconds(kernel32: ctypes.WinDLL, handle: int) -> float:
    creation, exit_time, kernel, user = FileTime(), FileTime(), FileTime(), FileTime()
    if not kernel32.GetProcessTimes(
        handle,
        ctypes.byref(creation),
        ctypes.byref(exit_time),
        ctypes.byref(kernel),
        ctypes.byref(user),
    ):
        raise ctypes.WinError(ctypes.get_last_error())

    def seconds(value: FileTime) -> float:
        return ((value.high << 32) | value.low) / 10_000_000

    return seconds(kernel) + seconds(user)


def sample(
    label: str,
    executable: Path,
    config: Path,
    index: int,
    settle_seconds: float,
    idle_seconds: float,
) -> dict[str, object]:
    user32 = ctypes.WinDLL("user32", use_last_error=True)
    kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)
    psapi = ctypes.WinDLL("psapi", use_last_error=True)
    started = time.perf_counter()
    process = subprocess.Popen(
        [str(executable), "-config", str(config), "-log-file", "-"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    try:
        window = None
        while time.perf_counter() - started < 15:
            if process.poll() is not None:
                raise RuntimeError(f"{label} exited before showing a window: {process.returncode}")
            window = visible_window_for_pid(user32, process.pid)
            if window:
                break
            time.sleep(0.025)
        if not window:
            raise RuntimeError(f"{label} did not show a window within 15 seconds")
        startup_ms = (time.perf_counter() - started) * 1000
        time.sleep(settle_seconds)

        handle = kernel32.OpenProcess(PROCESS_QUERY_INFORMATION | PROCESS_VM_READ, False, process.pid)
        if not handle:
            raise ctypes.WinError(ctypes.get_last_error())
        try:
            memory = ProcessMemoryCounters()
            memory.cb = ctypes.sizeof(memory)
            if not psapi.GetProcessMemoryInfo(handle, ctypes.byref(memory), memory.cb):
                raise ctypes.WinError(ctypes.get_last_error())
            cpu_start = process_cpu_seconds(kernel32, handle)
            wall_start = time.perf_counter()
            time.sleep(idle_seconds)
            wall_seconds = time.perf_counter() - wall_start
            idle_cpu = (process_cpu_seconds(kernel32, handle) - cpu_start) / wall_seconds * 100
        finally:
            kernel32.CloseHandle(handle)
        return {
            "label": label,
            "sample": index,
            "startup_window_ms": round(startup_ms, 2),
            "working_set_bytes": memory.working_set_size,
            "peak_working_set_bytes": memory.peak_working_set_size,
            "idle_cpu_one_core_percent": round(idle_cpu, 4),
        }
    finally:
        if process.poll() is None:
            process.kill()
        process.wait(timeout=10)


def medians(rows: list[dict[str, object]], label: str) -> dict[str, float]:
    selected = [row for row in rows if row["label"] == label]
    result: dict[str, float] = {}
    for metric in (
        "startup_window_ms",
        "working_set_bytes",
        "peak_working_set_bytes",
        "idle_cpu_one_core_percent",
    ):
        result[metric] = statistics.median(float(row[metric]) for row in selected)
    return result


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--baseline-exe", required=True, type=Path)
    parser.add_argument("--baseline-config", required=True, type=Path)
    parser.add_argument("--candidate-exe", required=True, type=Path)
    parser.add_argument("--candidate-config", required=True, type=Path)
    parser.add_argument("--baseline-commit", required=True)
    parser.add_argument("--candidate-commit", required=True)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--samples", type=int, default=3)
    parser.add_argument("--settle", type=float, default=2)
    parser.add_argument("--idle", type=float, default=3)
    args = parser.parse_args()
    if os.name != "nt":
        raise SystemExit("capture-phase15-process.py requires Windows")
    if args.samples < 3:
        raise SystemExit("--samples must be at least 3")
    for path in (args.baseline_exe, args.baseline_config, args.candidate_exe, args.candidate_config):
        if not path.is_file():
            raise SystemExit(f"missing input: {path}")

    rows: list[dict[str, object]] = []
    cases = [
        ("phase0", args.baseline_exe.resolve(), args.baseline_config.resolve()),
        ("candidate", args.candidate_exe.resolve(), args.candidate_config.resolve()),
    ]
    for index in range(args.samples):
        ordered = cases if index % 2 == 0 else list(reversed(cases))
        for label, executable, config in ordered:
            rows.append(sample(label, executable, config, index + 1, args.settle, args.idle))

    baseline = medians(rows, "phase0")
    candidate = medians(rows, "candidate")
    delta = {
        metric: (candidate[metric] - baseline[metric]) / baseline[metric] * 100
        if baseline[metric] != 0
        else 0
        for metric in baseline
    }
    report = {
        "schema_version": 1,
        "captured_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "baseline_commit": args.baseline_commit,
        "candidate_commit": args.candidate_commit,
        "protocol": f"{args.samples} interleaved samples; {args.settle}s settle; {args.idle}s idle sample; forced termination after measurement",
        "host": {"os": platform.platform(), "arch": platform.machine(), "cpu_count": os.cpu_count()},
        "artifacts_sha256": {
            "baseline_executable": sha256(args.baseline_exe),
            "baseline_config": sha256(args.baseline_config),
            "candidate_executable": sha256(args.candidate_exe),
            "candidate_config": sha256(args.candidate_config),
        },
        "samples": rows,
        "medians": {"phase0": baseline, "candidate": candidate, "delta_percent": delta},
    }
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report["medians"], indent=2))


if __name__ == "__main__":
    main()
