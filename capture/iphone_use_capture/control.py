"""Manage only this capture worker; never stop the receiver or a process tree."""
import argparse
import contextlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time
from urllib.request import ProxyHandler, build_opener

import psutil

OWNER = str(Path(__file__).resolve().parent)


def health(port):
    with build_opener(ProxyHandler({})).open(f"http://127.0.0.1:{port}/health", timeout=2) as response:
        value = json.load(response)
    if value.get("service") != "iphone-use-capture" or value.get("owner") != OWNER:
        raise RuntimeError("Port belongs to another service; refusing to manage it")
    return value


def owned_process(state):
    process = psutil.Process(state["pid"])
    if process.create_time() != state["created"]:
        raise RuntimeError("PID was reused; refusing to stop it")
    cmd = process.cmdline()
    if "iphone_use_capture" not in cmd or "-m" not in cmd:
        raise RuntimeError("Process identity does not match capture worker")
    return process


@contextlib.contextmanager
def lock(path):
    import msvcrt
    with path.open("a+b") as file:
        if file.tell() == 0:
            file.write(b"0")
            file.flush()
        file.seek(0)
        msvcrt.locking(file.fileno(), msvcrt.LK_NBLCK, 1)
        try:
            yield
        finally:
            file.seek(0)
            msvcrt.locking(file.fileno(), msvcrt.LK_UNLCK, 1)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("action", choices=("start", "status", "stop"))
    parser.add_argument("--runtime", type=Path, required=True)
    parser.add_argument("--uxplay-dir", type=Path)
    parser.add_argument("--port", type=int, default=8892)
    parser.add_argument("--name", default="iPhone Use")
    parser.add_argument("--receiver", choices=("attach", "start"), default="attach")
    args = parser.parse_args()
    runtime = args.runtime.resolve()
    runtime.mkdir(parents=True, exist_ok=True)
    state_file = runtime / "worker.json"
    with lock(runtime / "control.lock"):
        state = json.loads(state_file.read_text()) if state_file.exists() else None
        if args.action == "status":
            print(json.dumps(health(state["port"] if state else args.port)))
            return
        if args.action == "stop":
            if not state:
                raise RuntimeError("No owned worker state; refusing to stop a port occupant")
            try:
                process = owned_process(state)
            except psutil.NoSuchProcess:
                state_file.unlink()
                print('{"state":"already_stopped"}')
                return
            process.terminate()
            process.wait(10)
            state_file.unlink()
            print('{"state":"stopped", "receiver":"untouched"}')
            return
        if state:
            try:
                owned_process(state)
            except psutil.NoSuchProcess:
                state_file.unlink()
            else:
                raise RuntimeError("Worker already exists; use status or explicit stop")
        if not args.uxplay_dir:
            parser.error("start requires --uxplay-dir")
        receiver = args.uxplay_dir.resolve()
        if not (receiver / "uxplay-windows.exe").is_file():
            parser.error("uxplay-windows.exe not found")
        # Child binds HTTP before any pipe/config/receiver mutation. Port collisions fail.
        cmd = [sys.executable, "-m", "iphone_use_capture", "--port", str(args.port),
               "--name", args.name, "--uxplay-dir", str(receiver),
               "--data-dir", str(runtime), "--receiver", args.receiver]
        with (runtime / "capture.log").open("ab") as log:
            child = subprocess.Popen(cmd, stdin=subprocess.DEVNULL, stdout=log, stderr=log,
                                     creationflags=subprocess.CREATE_NO_WINDOW)
        # Only accept this child's actual Python worker, not an unrelated listener.
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            if child.poll() is not None:
                raise RuntimeError("Capture exited; inspect capture.log")
            try:
                value = health(args.port)
            except OSError:
                time.sleep(.1)
                continue
            descendants = {child.pid} | {p.pid for p in psutil.Process(child.pid).children(recursive=True)}
            if value["pid"] not in descendants:
                child.terminate()
                child.wait(10)
                raise RuntimeError("Port is already owned by another worker")
            process = psutil.Process(value["pid"])
            state_file.write_text(json.dumps({"pid":process.pid, "created":process.create_time(),
                                              "port":args.port, "owner":OWNER}), encoding="utf-8")
            print(json.dumps(value))
            return
        # Keep the process recorded for an explicit, identity-checked stop.
        state_file.write_text(json.dumps({"pid":child.pid, "created":psutil.Process(child.pid).create_time(),
                                          "port":args.port, "owner":OWNER}), encoding="utf-8")
        raise RuntimeError("Readiness timed out; inspect capture.log and run status/stop")


if __name__ == "__main__":
    main()
