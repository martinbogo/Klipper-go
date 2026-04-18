from __future__ import annotations

import json
import os
import time
from pathlib import Path

import paramiko

REMOTE_DIR = "/userdata/app/gk"
REMOTE_GKLIB = f"{REMOTE_DIR}/gklib"
REMOTE_HELPER = f"{REMOTE_DIR}/libc_helper.so"
REMOTE_GKLIB_STAGE = f"{REMOTE_DIR}/gklib.copilot.livecandidate"
REMOTE_HELPER_STAGE = f"{REMOTE_DIR}/libc_helper.so.copilot.livecandidate"
REMOTE_GKLIB_BACKUP = f"{REMOTE_DIR}/gklib.copilot.livebackup"
REMOTE_HELPER_BACKUP = f"{REMOTE_DIR}/libc_helper.so.copilot.livebackup"
REMOTE_SOCKET = "/tmp/unix_uds1"
REMOTE_LOG = "/tmp/rinkhals/gklib.log"
LOCAL_GKLIB = Path("gklib_uclibc_current")
LOCAL_HELPER = Path("internal/pkg/chelper/libc_helper.so")


def connect(timeout: float = 15.0) -> paramiko.SSHClient:
	client = paramiko.SSHClient()
	client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
	client.connect(
		os.environ["PRINTER_HOST"],
		username=os.environ["PRINTER_USER"],
		password=os.environ["PRINTER_PASSWORD"],
		timeout=timeout,
		banner_timeout=timeout,
		auth_timeout=timeout,
	)
	return client


def run(client: paramiko.SSHClient, cmd: str, timeout: int = 60) -> tuple[int, str, str]:
	stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout)
	out = stdout.read().decode("utf-8", "replace")
	err = stderr.read().decode("utf-8", "replace")
	code = stdout.channel.recv_exit_status()
	return code, out, err


def must_run(client: paramiko.SSHClient, cmd: str, timeout: int = 60) -> str:
	code, out, err = run(client, cmd, timeout)
	if code != 0:
		raise RuntimeError(f"command failed ({code}): {cmd}\nSTDOUT:\n{out}\nSTDERR:\n{err}")
	return out


def remote_json_request(client: paramiko.SSHClient, method: str, params: dict, request_id: int, timeout: int = 30) -> dict:
	payload = json.dumps({"id": request_id, "method": method, "params": params}, separators=(",", ":"))
	cmd = f"""python3 - <<'PY2'
import json
import socket
payload = {payload!r}.encode() + b"\\x03"
sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
sock.settimeout({timeout})
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
PY2"""
	code, out, err = run(client, cmd, timeout=timeout + 15)
	if code != 0:
		raise RuntimeError(f"request {method} failed ({code}) stderr={err!r} stdout={out!r}")
	return json.loads(out.strip())


def connect_with_retry(total_timeout: float = 420.0, delay: float = 5.0) -> paramiko.SSHClient:
	start = time.time()
	attempt = 0
	while time.time() - start < total_timeout:
		attempt += 1
		try:
			client = connect(timeout=15.0)
			code, _, _ = run(client, "true", timeout=10)
			if code == 0:
				print(json.dumps({"step": "ssh_reconnected", "attempt": attempt}), flush=True)
				return client
			client.close()
		except Exception as exc:
			print(json.dumps({"step": "ssh_retry", "attempt": attempt, "error": str(exc)}), flush=True)
		time.sleep(delay)
	raise RuntimeError("printer did not come back on SSH in time")


def main() -> int:
	if not LOCAL_GKLIB.exists():
		raise RuntimeError(f"missing local candidate: {LOCAL_GKLIB}")
	if not LOCAL_HELPER.exists():
		raise RuntimeError(f"missing local helper: {LOCAL_HELPER}")

	initial = connect()
	try:
		sftp = initial.open_sftp()
		try:
			sftp.put(str(LOCAL_GKLIB), REMOTE_GKLIB_STAGE)
			sftp.put(str(LOCAL_HELPER), REMOTE_HELPER_STAGE)
		finally:
			sftp.close()

		must_run(initial, f"chmod 755 {REMOTE_GKLIB_STAGE} {REMOTE_HELPER_STAGE}", timeout=30)
		print(json.dumps({"step": "staged"}), flush=True)

		verify_out = must_run(initial, f"md5sum {REMOTE_GKLIB_STAGE} {REMOTE_HELPER_STAGE}", timeout=30)
		print(json.dumps({"step": "staged_md5", "stdout": verify_out.strip()}), flush=True)

		must_run(
			initial,
			f"cp {REMOTE_GKLIB} {REMOTE_GKLIB_BACKUP} && "
			f"cp {REMOTE_HELPER} {REMOTE_HELPER_BACKUP} && "
			f"cp {REMOTE_GKLIB_STAGE} {REMOTE_GKLIB} && "
			f"cp {REMOTE_HELPER_STAGE} {REMOTE_HELPER} && "
			f"chmod 755 {REMOTE_GKLIB} {REMOTE_HELPER}",
			timeout=60,
		)
		live_md5 = must_run(initial, f"md5sum {REMOTE_GKLIB} {REMOTE_HELPER}", timeout=30)
		print(json.dumps({"step": "installed_md5", "stdout": live_md5.strip()}), flush=True)

		try:
			run(initial, "reboot", timeout=10)
		except Exception as exc:
			print(json.dumps({"step": "reboot_send_exception", "error": str(exc)}), flush=True)
	finally:
		try:
			initial.close()
		except Exception:
			pass

	client = connect_with_retry()
	try:
		time.sleep(20)
		state = {
			"socket": must_run(
				client,
				f"if test -S {REMOTE_SOCKET}; then echo socket-present; elif test -e {REMOTE_SOCKET}; then ls -l {REMOTE_SOCKET}; else echo socket-missing; fi",
				timeout=20,
			).strip(),
			"process": must_run(
				client,
				"ps | grep './gklib -a /tmp/unix_uds1 rinkhals_gklib.cfg' | grep -v grep || true",
				timeout=20,
			).strip(),
			"query": remote_json_request(client, "Query/K3cInfo", {}, 201, timeout=15),
		}
		print(json.dumps({"step": "post_reboot_state", "state": state}, indent=2), flush=True)

		g28w = remote_json_request(client, "gcode/script", {"script": "G28 W"}, 202, timeout=420)
		print(json.dumps({"step": "g28w_response", "response": g28w}, indent=2), flush=True)

		post_query = remote_json_request(client, "Query/K3cInfo", {}, 203, timeout=15)
		post_log = must_run(
			client,
			"grep -E 'web hook do script:G28 W|Unable to write tmc uart|project state: Shutdown|project state: Ready|message_ready|ACE: Connected|Homing failed' /tmp/rinkhals/gklib.log 2>/dev/null | tail -n 200 || tail -n 200 /tmp/rinkhals/gklib.log 2>/dev/null || true",
			timeout=30,
		)
		print(json.dumps({"step": "post_g28w_query", "response": post_query}, indent=2), flush=True)
		print(json.dumps({"step": "post_g28w_log", "stdout": post_log}, indent=2), flush=True)

		if "error" in g28w or not post_query.get("result", {}).get("ready", False):
			return 1
		return 0
	finally:
		client.close()


if __name__ == "__main__":
	raise SystemExit(main())
