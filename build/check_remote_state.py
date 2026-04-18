from __future__ import annotations

import json
import os

import paramiko


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
        cmd = r"""sh -lc '
    ps | grep -E "gklib|serial_shim" | grep -v grep || true
printf "__FILES__\n"
ls -l /tmp/unix_uds1 /tmp/gklib.restore.log /tmp/gklib_candidate_live.log 2>/dev/null || true
    printf "__RESTORE_LOG_TAIL__\n"
    tail -n 40 /tmp/gklib.restore.log 2>/dev/null || true
    printf "__CANDIDATE_LOG_TAIL__\n"
    tail -n 40 /tmp/gklib_candidate_live.log 2>/dev/null || true
printf "__QUERY__\n"
python3 - <<"PY"
import json
import socket
payload = json.dumps({"id": 1, "method": "Query/K3cInfo", "params": {}}).encode() + b"\x03"
try:
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.settimeout(5)
    sock.connect("/tmp/unix_uds1")
    sock.sendall(payload)
    data = b""
    while True:
        chunk = sock.recv(65536)
        if not chunk:
            break
        data += chunk
        if b"\x03" in data:
            data = data.split(b"\x03", 1)[0]
            break
    sock.close()
    print(data.decode("utf-8", "replace"))
except Exception as exc:
    print(json.dumps({"query_error": str(exc)}))
PY
'"""
        stdin, stdout, stderr = client.exec_command(cmd, timeout=25)
        out = stdout.read().decode("utf-8", "replace")
        err = stderr.read().decode("utf-8", "replace")
        code = stdout.channel.recv_exit_status()
        print(json.dumps({"code": code, "stdout": out, "stderr": err}, indent=2))
    finally:
        client.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
