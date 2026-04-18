from __future__ import annotations

import json
import os
import pathlib
import shlex
import time
from datetime import datetime

import paramiko

REMOTE_DIR = "/userdata/app/gk"
REMOTE_CFG = f"{REMOTE_DIR}/rinkhals_gklib.cfg"
REMOTE_TEST_DIR = f"{REMOTE_DIR}/candidate_live"
REMOTE_CANDIDATE = f"{REMOTE_TEST_DIR}/gklib_candidate"
REMOTE_HELPER = f"{REMOTE_TEST_DIR}/libc_helper.so"
REMOTE_CANDIDATE_TMP = f"{REMOTE_TEST_DIR}/gklib_candidate.upload"
REMOTE_HELPER_TMP = f"{REMOTE_TEST_DIR}/libc_helper.so.upload"
REMOTE_SOCKET = "/tmp/unix_uds1"
REMOTE_CANDIDATE_LOG = "/tmp/gklib_candidate_live.log"
REMOTE_RESTORE_LOG = "/tmp/gklib.restore.log"
LOCAL_ROOT = pathlib.Path(__file__).resolve().parents[1]
LOCAL_CANDIDATE = LOCAL_ROOT / "gklib_uclibc_current"
LOCAL_HELPER = LOCAL_ROOT / "internal/pkg/chelper/libc_helper.so"
ARTIFACT_DIR = LOCAL_ROOT / "build" / f"live_motion_{datetime.now().strftime('%Y%m%d_%H%M%S')}"
ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)
SUMMARY_PATH = ARTIFACT_DIR / "summary.json"

summary: dict[str, object] = {
    "artifact_dir": str(ARTIFACT_DIR),
    "steps": [],
    "commands": [],
    "ready": None,
    "restored_original": False,
}


def note(step: str, **kwargs):
    entry = {"step": step, **kwargs}
    summary["steps"].append(entry)
    print(json.dumps(entry, sort_keys=True), flush=True)


class RemoteRunner:
    def __init__(self, host: str, user: str, password: str):
        self.client = paramiko.SSHClient()
        self.client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        self.client.connect(
            host,
            username=user,
            password=password,
            timeout=10,
            banner_timeout=10,
            auth_timeout=10,
        )
        self.sftp = self.client.open_sftp()

    def close(self):
        self.sftp.close()
        self.client.close()

    def run(self, cmd: str, timeout: int = 30):
        stdin, stdout, stderr = self.client.exec_command(cmd, timeout=timeout)
        out = stdout.read().decode("utf-8", "replace")
        err = stderr.read().decode("utf-8", "replace")
        code = stdout.channel.recv_exit_status()
        return out, err, code

    def must_run(self, cmd: str, timeout: int = 30):
        out, err, code = self.run(cmd, timeout=timeout)
        if code != 0:
            raise RuntimeError(f"command failed ({code}): {cmd}\nSTDOUT:\n{out}\nSTDERR:\n{err}")
        return out

    def upload(self, local_path: pathlib.Path, remote_path: str, mode: int):
        self.sftp.put(str(local_path), remote_path)
        self.must_run(f"chmod {mode:o} {shlex.quote(remote_path)}")

    def fetch_if_exists(self, remote_path: str, local_path: pathlib.Path):
        try:
            self.sftp.get(remote_path, str(local_path))
            return True
        except FileNotFoundError:
            return False


def stop_live_service(remote: RemoteRunner):
    remote.run(
        "killall gklib 2>/dev/null || true; "
        "pkill -f '/userdata/app/gk/gklib_candidate' 2>/dev/null || true; "
        "pkill -f '/userdata/app/gk/candidate_live/gklib_candidate' 2>/dev/null || true; "
        "killall serial_shim 2>/dev/null || true"
    )


def start_background(remote: RemoteRunner, cmd: str, timeout: int = 30) -> int:
    out = remote.must_run(cmd, timeout=timeout).strip().splitlines()
    if not out:
        raise RuntimeError(f"no pid returned for background command: {cmd}")
    return int(out[-1].strip())


def start_original(remote: RemoteRunner) -> int:
    cmd = (
        f"sh -lc 'cd {REMOTE_DIR} && export LD_LIBRARY_PATH={REMOTE_DIR}:$LD_LIBRARY_PATH; "
        f"nohup ./gklib -a {REMOTE_SOCKET} rinkhals_gklib.cfg >{REMOTE_RESTORE_LOG} 2>&1 & echo $!'"
    )
    return start_background(remote, cmd)


def start_candidate(remote: RemoteRunner) -> int:
    cmd = (
        f"sh -lc 'cd {REMOTE_DIR} && export LD_LIBRARY_PATH={REMOTE_TEST_DIR}:{REMOTE_DIR}:$LD_LIBRARY_PATH; "
        f"nohup {REMOTE_CANDIDATE} -a {REMOTE_SOCKET} rinkhals_gklib.cfg >{REMOTE_CANDIDATE_LOG} 2>&1 & echo $!'"
    )
    return start_background(remote, cmd)


def process_alive(remote: RemoteRunner, pid: int) -> bool:
    _, _, code = remote.run(f"kill -0 {pid} 2>/dev/null")
    return code == 0


def detect_remote_python(remote: RemoteRunner) -> str:
    out = remote.must_run("sh -lc 'command -v python3 || command -v python'", timeout=10).strip()
    if not out:
        raise RuntimeError("remote python interpreter not found")
    return out.splitlines()[-1].strip()


def socket_request(remote: RemoteRunner, python_bin: str, method: str, params: dict | None = None, timeout: int = 60, request_id: int = 1):
    payload = json.dumps({"id": request_id, "method": method, "params": params or {}}, separators=(",", ":"))
    remote_script = f"""
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
"""
    cmd = f"{shlex.quote(python_bin)} - <<'PY'\n{remote_script}\nPY"
    out, err, code = remote.run(cmd, timeout=timeout + 20)
    if code != 0:
        raise RuntimeError(f"socket request failed ({method}) code={code}\nSTDOUT:\n{out}\nSTDERR:\n{err}")
    text = out.strip()
    if not text:
        raise RuntimeError(f"empty response for {method}")
    return json.loads(text)


def query_toolhead(remote: RemoteRunner, python_bin: str, request_id: int):
    response = socket_request(
        remote,
        python_bin,
        "objects/query",
        {"objects": {"toolhead": None}},
        timeout=30,
        request_id=request_id,
    )
    result = response.get("result", {})
    status = result.get("status", {})
    return status.get("toolhead", {})


def tail_remote_log(remote: RemoteRunner, path: str, lines: int = 120) -> str:
    out, err, code = remote.run(f"tail -n {lines} {shlex.quote(path)}", timeout=20)
    if code != 0:
        return f"<tail failed for {path}>\nSTDOUT:\n{out}\nSTDERR:\n{err}"
    return out


def main() -> int:
    host = os.environ["PRINTER_HOST"]
    user = os.environ["PRINTER_USER"]
    password = os.environ["PRINTER_PASSWORD"]
    remote = RemoteRunner(host, user, password)
    python_bin = None
    candidate_pid = None
    request_id = 10

    try:
        if not LOCAL_CANDIDATE.exists():
            raise RuntimeError(f"missing local candidate binary: {LOCAL_CANDIDATE}")
        if not LOCAL_HELPER.exists():
            raise RuntimeError(f"missing local helper library: {LOCAL_HELPER}")

        python_bin = detect_remote_python(remote)
        note("remote_python", path=python_bin)

        stop_live_service(remote)
        time.sleep(2.0)

        remote.must_run(f"mkdir -p {shlex.quote(REMOTE_TEST_DIR)}")
        remote.must_run(
            f"rm -f {shlex.quote(REMOTE_CANDIDATE_TMP)} {shlex.quote(REMOTE_HELPER_TMP)}"
        )
        remote.upload(LOCAL_CANDIDATE, REMOTE_CANDIDATE_TMP, 0o755)
        remote.upload(LOCAL_HELPER, REMOTE_HELPER_TMP, 0o755)
        remote.must_run(
            f"mv -f {shlex.quote(REMOTE_CANDIDATE_TMP)} {shlex.quote(REMOTE_CANDIDATE)} && "
            f"mv -f {shlex.quote(REMOTE_HELPER_TMP)} {shlex.quote(REMOTE_HELPER)}"
        )
        note("uploaded_candidate", remote_candidate=REMOTE_CANDIDATE, remote_helper=REMOTE_HELPER)

        candidate_pid = start_candidate(remote)
        note("started_candidate", pid=candidate_pid)

        ready_info = None
        for attempt in range(1, 31):
            time.sleep(2.0)
            alive = process_alive(remote, candidate_pid)
            try:
                request_id += 1
                response = socket_request(remote, python_bin, "Query/K3cInfo", {}, timeout=20, request_id=request_id)
                result = response.get("result", {})
                ready_info = result
                note("ready_poll", attempt=attempt, alive=alive, state=result.get("state"), ready=result.get("ready"), state_message=result.get("state_message"))
                if result.get("ready") is True:
                    summary["ready"] = result
                    break
            except Exception as exc:
                note("ready_poll_error", attempt=attempt, alive=alive, error=str(exc))
                if not alive:
                    break
        if not ready_info or ready_info.get("ready") is not True:
            raise RuntimeError("candidate did not reach ready state")

        request_id += 1
        info_response = socket_request(remote, python_bin, "info", {"client_info": {"program": "copilot-live-motion-test"}}, timeout=20, request_id=request_id)
        note("info_response", result=info_response.get("result", {}))

        request_id += 1
        toolhead_before = query_toolhead(remote, python_bin, request_id)
        note("toolhead_before", status=toolhead_before)

        command_plan = [
            ("home_x", "G28 X\nM400", 180),
            ("home_y", "G28 Y\nM400", 180),
            ("home_z", "G28 Z\nM400", 240),
            ("leviq3_probe", "LEVIQ3_PROBE\nM400", 1800),
        ]

        for name, script, timeout in command_plan:
            request_id += 1
            response = socket_request(remote, python_bin, "gcode/script", {"script": script}, timeout=timeout, request_id=request_id)
            result = response.get("result")
            error = response.get("error")
            request_id += 1
            toolhead_status = query_toolhead(remote, python_bin, request_id)
            command_entry = {
                "name": name,
                "script": script,
                "timeout": timeout,
                "response": result,
                "error": error,
                "toolhead": toolhead_status,
            }
            summary["commands"].append(command_entry)
            note("command_complete", **command_entry)
            if error is not None:
                raise RuntimeError(f"command {name} failed: {error}")

        note("candidate_test_complete")
        return 0
    finally:
        try:
            if remote.fetch_if_exists(REMOTE_CANDIDATE_LOG, ARTIFACT_DIR / "gklib_candidate_live.log"):
                summary["candidate_log"] = str(ARTIFACT_DIR / "gklib_candidate_live.log")
            summary["candidate_log_tail"] = tail_remote_log(remote, REMOTE_CANDIDATE_LOG)
        except Exception as exc:
            summary["candidate_log_fetch_error"] = str(exc)

        try:
            stop_live_service(remote)
            time.sleep(1.0)
            restore_pid = start_original(remote)
            note("started_original", pid=restore_pid)
            restored_ready = None
            if python_bin is None:
                python_bin = detect_remote_python(remote)
            for attempt in range(1, 21):
                time.sleep(2.0)
                try:
                    request_id += 1
                    response = socket_request(remote, python_bin, "Query/K3cInfo", {}, timeout=20, request_id=request_id)
                    result = response.get("result", {})
                    note("restore_poll", attempt=attempt, state=result.get("state"), ready=result.get("ready"), state_message=result.get("state_message"))
                    if result.get("ready") is True:
                        restored_ready = result
                        break
                except Exception as exc:
                    note("restore_poll_error", attempt=attempt, error=str(exc))
            summary["restored_original"] = restored_ready is not None
            summary["restored_ready_info"] = restored_ready
            remote.fetch_if_exists(REMOTE_RESTORE_LOG, ARTIFACT_DIR / "gklib_restore.log")
            summary["restore_log_tail"] = tail_remote_log(remote, REMOTE_RESTORE_LOG)
        except Exception as exc:
            summary["restore_error"] = str(exc)
            note("restore_error", error=str(exc))
        finally:
            SUMMARY_PATH.write_text(json.dumps(summary, indent=2, sort_keys=True))
            print(f"SUMMARY_PATH={SUMMARY_PATH}", flush=True)
            remote.close()


if __name__ == "__main__":
    raise SystemExit(main())
