from __future__ import annotations

import json
import os
import shlex
import time

import paramiko

REMOTE_DIR = "/userdata/app/gk"
REMOTE_SOCKET = "/tmp/unix_uds1"
REMOTE_RESTORE_LOG = "/tmp/gklib.restore.log"


def remote_json_request(client: paramiko.SSHClient, method: str, params: dict | None = None) -> dict:
    payload = json.dumps({"id": 1, "method": method, "params": params or {}}, separators=(",", ":"))
    script = f"""python3 - <<'PY'
import json
import socket
payload = {payload!r}.encode() + b"\\x03"
sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
sock.settimeout(5)
sock.connect({REMOTE_SOCKET!r})
sock.sendall(payload)
data = b""
while True:
    chunk = sock.recv(65536)
    if not chunk:
        break
    data += chunk
    if b"\\x03" in data:
        data = data.split(b"\\x03", 1)[0]
        break
sock.close()
print(data.decode("utf-8", "replace"))
PY"""
    stdin, stdout, stderr = client.exec_command(script, timeout=15)
    out = stdout.read().decode("utf-8", "replace").strip()
    err = stderr.read().decode("utf-8", "replace")
    code = stdout.channel.recv_exit_status()
    if code != 0:
        raise RuntimeError(f"request {method} failed ({code}) stderr={err!r} stdout={out!r}")
    return json.loads(out)


def main() -> int:
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(
        os.environ["PRINTER_HOST"],
        username=os.environ["PRINTER_USER"],
        password=os.environ["PRINTER_PASSWORD"],
        timeout=10,
        banner_timeout=10,
        auth_timeout=10,
    )
    try:
        cmd = (
            "killall gklib_candidate 2>/dev/null || true; "
            "pkill -f '/userdata/app/gk/candidate_live/gklib_candidate' 2>/dev/null || true; "
            "killall gklib 2>/dev/null || true; "
            "killall serial_shim 2>/dev/null || true; "
            f"rm -f {shlex.quote(REMOTE_SOCKET)}; "
            f"cd {shlex.quote(REMOTE_DIR)} && export LD_LIBRARY_PATH={REMOTE_DIR}:$LD_LIBRARY_PATH; "
            f"nohup ./gklib -a {shlex.quote(REMOTE_SOCKET)} rinkhals_gklib.cfg >{shlex.quote(REMOTE_RESTORE_LOG)} 2>&1 & echo $!"
        )
        stdin, stdout, stderr = client.exec_command(f"sh -lc {shlex.quote(cmd)}", timeout=20)
        out = stdout.read().decode("utf-8", "replace").strip()
        err = stderr.read().decode("utf-8", "replace")
        code = stdout.channel.recv_exit_status()
        if code != 0:
            raise RuntimeError(f"restore start failed ({code}) stderr={err!r} stdout={out!r}")
        pid = int(out.splitlines()[-1])

        info_result = None
        info_error = None
        for attempt in range(1, 16):
            time.sleep(2)
            try:
                info_result = remote_json_request(client, "info", {"client_info": {"program": "copilot-restore-check", "version": "1"}})
                print(json.dumps({"step": "info_poll", "attempt": attempt, "response": info_result}, indent=2))
                break
            except Exception as exc:
                info_error = str(exc)
                print(json.dumps({"step": "info_poll_error", "attempt": attempt, "error": info_error}, indent=2))

        stdin, stdout, stderr = client.exec_command("ps | grep -E 'gklib|serial_shim' | grep -v grep || true", timeout=10)
        ps_out = stdout.read().decode("utf-8", "replace")
        ps_err = stderr.read().decode("utf-8", "replace")
        ps_code = stdout.channel.recv_exit_status()

        stdin, stdout, stderr = client.exec_command("tail -n 40 /tmp/gklib.restore.log 2>/dev/null || true", timeout=10)
        log_out = stdout.read().decode("utf-8", "replace")
        log_err = stderr.read().decode("utf-8", "replace")
        log_code = stdout.channel.recv_exit_status()

        print(json.dumps({
            "started_pid": pid,
            "info_result": info_result,
            "info_error": info_error,
            "ps_code": ps_code,
            "ps_stdout": ps_out,
            "ps_stderr": ps_err,
            "restore_log_code": log_code,
            "restore_log_stdout": log_out,
            "restore_log_stderr": log_err,
        }, indent=2))
    finally:
        client.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
