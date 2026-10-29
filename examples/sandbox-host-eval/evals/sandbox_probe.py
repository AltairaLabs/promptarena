#!/usr/bin/env python3
"""Host-side eval: score a session's sandbox container.

Arena runs this on the host after the agent finishes, before the sandbox
container is removed. The request arrives as JSON on stdin; the sandbox is
described under context.metadata.arena:

    {"params": {...},
     "context": {"metadata": {"arena": {
        "run_id": "...", "scenario_id": "...", "trial_index": 0,
        "labels": {...}, "artifacts_dir": "...",
        "sandboxes": {"sandbox": {
            "container_id": "...",
            "ports": {"5000": {"host_port": 54321,
                               "url": "http://localhost:54321"}}}}}}}}

It writes {"score": <float>, "detail": "...", "data": {...}} to stdout.
Logs go to stderr. Uses only the Python standard library and the docker CLI.
"""
import json
import subprocess
import sys
import time
import urllib.request

# A stand-in for "start the app fresh": the probe first kills anything the
# agent left on the port, then serves a fixed response with nc. A real harness
# would start its app here instead (e.g. `node app.js` in /app).
APP_CMD = "while true; do printf 'HTTP/1.0 200 OK\\r\\n\\r\\napp-ok' | nc -l -p {port}; done"


def docker(*args):
    return subprocess.run(["docker", *args], capture_output=True, text=True, timeout=30)


def http_get(url, wait_s=10):
    deadline = time.time() + wait_s
    while True:
        try:
            with urllib.request.urlopen(url, timeout=2) as resp:
                return resp.status, resp.read().decode(errors="replace")
        except OSError:
            if time.time() > deadline:
                raise
            time.sleep(0.3)


def main():
    req = json.load(sys.stdin)
    params = req.get("params") or {}
    arena = ((req.get("context") or {}).get("metadata") or {}).get("arena") or {}
    sandbox = (arena.get("sandboxes") or {}).get("sandbox") or {}
    cid = sandbox.get("container_id")
    if not cid:
        print(json.dumps({"score": 0, "detail": "no sandbox container in context.metadata.arena"}))
        return

    ctr_port = str(params.get("container_port", "5000"))
    port = (sandbox.get("ports") or {}).get(ctr_port) or {}
    data = {
        "run_id": arena.get("run_id"),
        "scenario_id": arena.get("scenario_id"),
        "trial_index": arena.get("trial_index"),
        "container_id": cid,
        "host_port": port.get("host_port"),
    }

    inspect = docker("inspect", "-f", "{{.State.Running}}", cid)
    data["container_alive"] = inspect.returncode == 0 and inspect.stdout.strip() == "true"

    cat = docker("exec", cid, "cat", params.get("file", "/workspace/hello.txt"))
    data["file_ok"] = cat.returncode == 0
    data["file_contents"] = cat.stdout.strip()

    data["http_ok"] = False
    if port.get("url"):
        # pkill runs directly, not under sh -c: pkill never matches itself,
        # but it would match a shell whose command line names the pattern.
        docker("exec", cid, "pkill", "-f", f"nc -l -p {ctr_port}")
        docker("exec", "-d", cid, "sh", "-c", APP_CMD.format(port=ctr_port))
        try:
            status, body = http_get(port["url"])
            data["http_ok"] = status == 200
            data["http_body"] = body.strip()
        except OSError as err:
            data["http_error"] = str(err)

    checks = ["container_alive", "file_ok", "http_ok"]
    passed = sum(1 for c in checks if data[c])
    print(json.dumps({
        "score": passed / len(checks),
        "detail": f"{passed}/{len(checks)} sandbox checks passed",
        "data": data,
    }))


if __name__ == "__main__":
    main()
