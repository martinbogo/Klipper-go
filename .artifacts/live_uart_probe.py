from __future__ import annotations

import json
import os
import socket
import sys
import time

SOCKET_PATH = "/tmp/unix_uds1"
DELIM = b"\x03"


def request(method: str, params: dict, timeout: float = 60.0, request_id: int = 1) -> dict:
    payload = json.dumps({"id": request_id, "method": method, "params": params}, separators=(",", ":")).encode() + DELIM
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.settimeout(timeout)
    sock.connect(SOCKET_PATH)
    try:
        sock.sendall(payload)
        data = b""
        while True:
            chunk = sock.recv(65536)
            if not chunk:
                break
            data += chunk
            if DELIM in data:
                data = data.split(DELIM, 1)[0]
                break
    finally:
        sock.close()
    if not data:
        raise RuntimeError(f"empty response for {method}")
    return json.loads(data.decode("utf-8", "replace"))


def main() -> int:
    loops = int(os.environ.get("UART_PROBE_LOOPS", "10"))
    pause_s = float(os.environ.get("UART_PROBE_PAUSE", "0.2"))
    scripts = [
        "DUMP_TMC STEPPER=stepper_x REGISTER=IFCNT\nM400",
        "DUMP_TMC STEPPER=stepper_y REGISTER=IFCNT\nM400",
        "DUMP_TMC STEPPER=stepper_z REGISTER=IFCNT\nM400",
        "DUMP_TMC STEPPER=stepper_x REGISTER=GCONF\nM400",
        "DUMP_TMC STEPPER=stepper_y REGISTER=GCONF\nM400",
    ]
    results: list[dict] = []
    req_id = 1000
    start = time.time()
    ready = request("Query/K3cInfo", {}, timeout=20, request_id=req_id)
    results.append({"phase": "ready", "response": ready})
    req_id += 1
    for loop in range(loops):
        for script in scripts:
            entry = {"loop": loop, "script": script}
            try:
                resp = request("gcode/script", {"script": script}, timeout=60, request_id=req_id)
                entry["response"] = resp
            except Exception as exc:
                entry["error"] = str(exc)
            results.append(entry)
            req_id += 1
            if pause_s:
                time.sleep(pause_s)
    end = time.time()
    print(json.dumps({
        "loops": loops,
        "elapsed_s": round(end - start, 3),
        "results": results,
    }, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
