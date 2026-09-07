#!/usr/bin/env python3
"""IDE-spawn-shaped MCP client: initialize + tools/list on stdio. No TTY."""

from __future__ import annotations

import json
import os
import subprocess
import sys
import time


def _recv(proc: subprocess.Popen[bytes], deadline: float) -> dict:
    while True:
        if time.time() > deadline:
            raise TimeoutError("timeout waiting for MCP reply")
        line = proc.stdout.readline()
        if not line:
            err = proc.stderr.read() if proc.stderr else b""
            raise EOFError(err.decode("utf-8", "replace"))
        line = line.strip()
        if not line:
            continue
        msg = json.loads(line)
        if msg.get("method") and "result" not in msg and "error" not in msg:
            continue
        return msg


def main() -> int:
    if len(sys.argv) < 2:
        print("usage: mcp_list.py [--timeout N] -- <cmd> [args...]", file=sys.stderr)
        return 2
    timeout = 20.0
    args = sys.argv[1:]
    if args and args[0] == "--timeout":
        timeout = float(args[1])
        args = args[2:]
    if args and args[0] == "--":
        args = args[1:]
    if not args:
        print("missing command after --", file=sys.stderr)
        return 2

    proc = subprocess.Popen(
        args,
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        bufsize=0,
    )
    assert proc.stdin is not None and proc.stdout is not None
    deadline = time.time() + timeout
    try:
        for req in (
            {
                "jsonrpc": "2.0",
                "id": 1,
                "method": "initialize",
                "params": {
                    "protocolVersion": "2024-11-05",
                    "capabilities": {},
                    "clientInfo": {"name": "deadbugz-verify", "version": "0"},
                },
            },
            {"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}},
            {"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}},
        ):
            proc.stdin.write((json.dumps(req) + "\n").encode())
            proc.stdin.flush()
            if "id" not in req:
                continue
            msg = _recv(proc, deadline)
            if req["id"] == 2:
                json.dump(msg, sys.stdout)
                sys.stdout.write("\n")
                if msg.get("error"):
                    return 1
                return 0
        return 1
    finally:
        proc.kill()
        try:
            proc.wait(timeout=2)
        except subprocess.TimeoutExpired:
            os.kill(proc.pid, 9)


if __name__ == "__main__":
    raise SystemExit(main())
