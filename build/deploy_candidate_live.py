#!/usr/bin/env python3

from __future__ import annotations

import os
import time
from pathlib import Path

import paramiko

HOST = os.environ["PRINTER_HOST"]
USER = os.environ["PRINTER_USER"]
PASSWORD = os.environ["PRINTER_PASSWORD"]

REMOTE_GK_DIR = "/userdata/app/gk"
REMOTE_GKLIB = f"{REMOTE_GK_DIR}/gklib"
REMOTE_HELPER = f"{REMOTE_GK_DIR}/libc_helper.so"
REMOTE_GKLIB_BACKUP = f"{REMOTE_GK_DIR}/gklib.copilot.livebackup"
REMOTE_HELPER_BACKUP = f"{REMOTE_GK_DIR}/libc_helper.so.copilot.livebackup"
REMOTE_GKLIB_STAGE = f"{REMOTE_GK_DIR}/gklib.copilot.livecandidate"
REMOTE_HELPER_STAGE = f"{REMOTE_GK_DIR}/libc_helper.so.copilot.livecandidate"
LOCAL_CANDIDATE = Path("gklib_uclibc_current")
LOCAL_HELPER = Path("internal/pkg/chelper/libc_helper.so")
RINKHALS_STOP = "/useremain/rinkhals/.current/stop.sh"
RINKHALS_START = "sh -lc 'cd /useremain/rinkhals && ./start-rinkhals.sh'"
LOG_PATH = "/tmp/rinkhals/gklib.log"
SOCKET_PATH = "/tmp/unix_uds1"


def connect_ready(timeout: float = 300.0, connect_timeout: float = 15.0) -> paramiko.SSHClient:
    start = time.time()
    last_error: Exception | None = None
    while time.time() - start < timeout:
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        try:
            client.connect(
                HOST,
                username=USER,
                password=PASSWORD,
                timeout=connect_timeout,
                banner_timeout=connect_timeout,
                auth_timeout=connect_timeout,
            )
            stdin, stdout, stderr = client.exec_command("true", timeout=connect_timeout)
            stdout.read()
            stderr.read()
            return client
        except Exception as exc:
            last_error = exc
            try:
                client.close()
            except Exception:
                pass
            time.sleep(3)
    raise RuntimeError(f"command-ready SSH did not return in time: {last_error!r}")


def run(client: paramiko.SSHClient, cmd: str, timeout: int = 120) -> tuple[str, str, int]:
    stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode("utf-8", "replace")
    err = stderr.read().decode("utf-8", "replace")
    code = stdout.channel.recv_exit_status()
    return out, err, code


def must_run(client: paramiko.SSHClient, cmd: str, timeout: int = 120) -> str:
    out, err, code = run(client, cmd, timeout)
    if code != 0:
        raise RuntimeError(
            f"command failed ({code}): {cmd}\nSTDOUT:\n{out}\nSTDERR:\n{err}"
        )
    return out


def print_result(label: str, out: str, err: str, code: int) -> None:
    print(f">>> {label} [code={code}]")
    print(out[:20000].strip())
    if err.strip():
        print("stderr:")
        print(err[:4000].strip())


def health_snapshot(client: paramiko.SSHClient) -> list[tuple[str, str, str, int]]:
    commands = [
        f"if test -S {SOCKET_PATH}; then echo socket-present; elif test -e {SOCKET_PATH}; then ls -l {SOCKET_PATH}; else echo socket-missing; fi",
        "ps | grep './gklib -a /tmp/unix_uds1 rinkhals_gklib.cfg' | grep -v grep",
        f"grep -E 'Loaded MCU|Timeout on connect|Serial connection closed|Unable to obtain|message_ready|project state: Ready|ACE: Connected' {LOG_PATH} | tail -n 100 || true",
    ]
    return [(cmd, *run(client, cmd, 120)) for cmd in commands]


def healthy(results: list[tuple[str, str, str, int]]) -> bool:
    socket_out = results[0][1].strip()
    process_out = results[1][1].strip()
    log_out = results[2][1]
    return (
        socket_out == "socket-present"
        and bool(process_out)
        and "Loaded MCU 'mcu'" in log_out
        and "Loaded MCU 'nozzle_mcu'" in log_out
        and ("message_ready" in log_out or "project state: Ready" in log_out)
    )


def restart_wrapper(client: paramiko.SSHClient, prefix: str = "") -> None:
    for cmd in [RINKHALS_STOP, RINKHALS_START]:
        out, err, code = run(client, cmd, 240)
        print_result(f"{prefix}{cmd}", out, err, code)
        time.sleep(5)


def rollback(client: paramiko.SSHClient) -> None:
    must_run(
        client,
        f"cp {REMOTE_GKLIB_BACKUP} {REMOTE_GKLIB} && "
        f"cp {REMOTE_HELPER_BACKUP} {REMOTE_HELPER} && "
        f"chmod 755 {REMOTE_GKLIB} {REMOTE_HELPER}",
    )
    restart_wrapper(client, prefix="ROLLBACK ")


def main() -> int:
    client = connect_ready()
    sftp = client.open_sftp()
    rollback_needed = False
    try:
        sftp.put(str(LOCAL_CANDIDATE), REMOTE_GKLIB_STAGE)
        sftp.put(str(LOCAL_HELPER), REMOTE_HELPER_STAGE)
        must_run(client, f"chmod 755 {REMOTE_GKLIB_STAGE} {REMOTE_HELPER_STAGE}")
        must_run(client, f"cp {REMOTE_GKLIB} {REMOTE_GKLIB_BACKUP} && cp {REMOTE_HELPER} {REMOTE_HELPER_BACKUP}")
        must_run(client, f"cp {REMOTE_GKLIB_STAGE} {REMOTE_GKLIB} && cp {REMOTE_HELPER_STAGE} {REMOTE_HELPER}")
        must_run(client, f"chmod 755 {REMOTE_GKLIB} {REMOTE_HELPER}")
        must_run(client, f"mkdir -p /tmp/rinkhals && : > {LOG_PATH}")
        rollback_needed = True

        restart_wrapper(client)
        time.sleep(25)

        results = health_snapshot(client)
        for cmd, out, err, code in results:
            print_result(cmd, out, err, code)

        if not healthy(results):
            raise RuntimeError("candidate did not reach healthy live state")

        print("LIVE_CANDIDATE_DEPLOY_OK")
        rollback_needed = False
        return 0
    finally:
        try:
            sftp.close()
        except Exception:
            pass
        if rollback_needed:
            try:
                rollback(client)
            except Exception as exc:
                print(f"ROLLBACK_FAILED: {exc}")
        try:
            client.close()
        except Exception:
            pass


if __name__ == "__main__":
    raise SystemExit(main())
