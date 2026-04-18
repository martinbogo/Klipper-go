#!/usr/bin/env python3

from __future__ import annotations

import os
import time
from pathlib import Path

import paramiko

REMOTE_GK_DIR = "/userdata/app/gk"
REMOTE_GKLIB = f"{REMOTE_GK_DIR}/gklib"
REMOTE_HELPER = f"{REMOTE_GK_DIR}/libc_helper.so"
REMOTE_GKLIB_BACKUP = f"{REMOTE_GK_DIR}/gklib.copilot.restore"
REMOTE_HELPER_BACKUP = f"{REMOTE_GK_DIR}/libc_helper.so.copilot.restore"
REMOTE_GKLIB_SWAP = f"{REMOTE_GK_DIR}/gklib.copilot.swap"
REMOTE_HELPER_SWAP = f"{REMOTE_GK_DIR}/libc_helper.so.copilot.swap"
RINKHALS_STOP = "/useremain/rinkhals/.current/stop.sh"
RINKHALS_START = "cd /useremain/rinkhals && ./start-rinkhals.sh"
LOCAL_CANDIDATE = Path("gklib_uclibc_current")
LOCAL_HELPER = Path("internal/pkg/chelper/libc_helper.so")
RINKHALS_GKLIB_LOG = "/tmp/rinkhals/gklib.log"
POST_RESTORE_SOCKET = "/tmp/unix_uds1"


def run(client: paramiko.SSHClient, cmd: str, timeout: int = 120) -> tuple[str, str, int]:
    stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode("utf-8", "replace")
    err = stderr.read().decode("utf-8", "replace")
    code = stdout.channel.recv_exit_status()
    return out, err, code


def must_run(client: paramiko.SSHClient, cmd: str, timeout: int = 120) -> str:
    out, err, code = run(client, cmd, timeout=timeout)
    if code != 0:
        raise RuntimeError(
            f"command failed ({code}): {cmd}\nSTDOUT:\n{out}\nSTDERR:\n{err}"
        )
    return out


def close_client(client: paramiko.SSHClient | None) -> None:
    if client is None:
        return
    try:
        client.close()
    except Exception:
        pass


def connect_ssh_with_retry(
    host: str,
    user: str,
    password: str,
    timeout: float,
    *,
    connect_timeout: float = 10.0,
    retry_delay: float = 3.0,
) -> paramiko.SSHClient:
    start = time.time()
    while time.time() - start < timeout:
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        try:
            client.connect(
                host,
                username=user,
                password=password,
                timeout=connect_timeout,
                banner_timeout=connect_timeout,
                auth_timeout=connect_timeout,
            )
            stdin, stdout, stderr = client.exec_command("true", timeout=connect_timeout)
            stdout.read()
            stderr.read()
            return client
        except Exception:
            try:
                client.close()
            except Exception:
                pass
            time.sleep(retry_delay)
    raise RuntimeError("host did not become SSH-connectable in time")


def socket_status_cmd(path: str) -> str:
    return (
        f"if test -S {path}; then echo socket-present; "
        f"elif test -e {path}; then ls -l {path}; "
        f"else echo socket-missing; fi"
    )


def print_cmd_result(
    prefix: str,
    cmd: str,
    out: str,
    err: str,
    code: int,
    *,
    out_limit: int,
    err_limit: int,
) -> None:
    print(f">>> {prefix}{cmd} [code={code}]")
    print(out[:out_limit].strip())
    if err.strip():
        print("stderr:")
        print(err[:err_limit].strip())


def send_reboot(client: paramiko.SSHClient, label: str) -> None:
    try:
        stdin, stdout, stderr = client.exec_command("reboot", timeout=10)
        try:
            stdout.read()
            stderr.read()
        except Exception:
            pass
    except Exception as exc:
        print(f"{label}: {exc}")


def collect_post_restore_results(client: paramiko.SSHClient) -> list[tuple[str, str, str, int]]:
    results: list[tuple[str, str, str, int]] = []
    for cmd in [
        socket_status_cmd(POST_RESTORE_SOCKET),
        "ps | grep './gklib -a /tmp/unix_uds1 rinkhals_gklib.cfg' | grep -v grep",
        "grep -E 'Loaded MCU|Timeout on connect|Serial connection closed|Unable to obtain|message_ready|project state: Ready' /tmp/rinkhals/gklib.log | tail -n 60 || true",
    ]:
        out, err, code = run(client, cmd, timeout=60)
        results.append((cmd, out, err, code))
    return results


def print_post_restore_results(results: list[tuple[str, str, str, int]]) -> None:
    for cmd, out, err, code in results:
        print_cmd_result(
            "post-restore ",
            cmd,
            out,
            err,
            code,
            out_limit=16000,
            err_limit=4000,
        )


def post_restore_healthy(results: list[tuple[str, str, str, int]]) -> bool:
    if len(results) < 3:
        return False
    socket_out = results[0][1]
    process_out = results[1][1]
    log_out = results[2][1]
    return (
        socket_out.strip() == "socket-present"
        and bool(process_out.strip())
        and "Loaded MCU 'mcu'" in log_out
        and "Loaded MCU 'nozzle_mcu'" in log_out
    )


def wait_for_post_restore_health(
    host: str,
    user: str,
    password: str,
    timeout: float,
) -> list[tuple[str, str, str, int]]:
    start = time.time()
    last_results: list[tuple[str, str, str, int]] = []
    recovery_reboot_sent = False
    while time.time() - start < timeout:
        client: paramiko.SSHClient | None = None
        try:
            remaining = timeout - (time.time() - start)
            client = connect_ssh_with_retry(
                host,
                user,
                password,
                max(remaining, 30.0),
                connect_timeout=15,
            )
            time.sleep(20)
            last_results = collect_post_restore_results(client)
            print_post_restore_results(last_results)
            if post_restore_healthy(last_results):
                return last_results
            if not recovery_reboot_sent:
                print("post-restore baseline not healthy yet; sending one recovery reboot")
                send_reboot(client, "post_restore_reboot_error")
                recovery_reboot_sent = True
                close_client(client)
                client = None
                continue
        except Exception as exc:
            print(f"post_restore_probe_error: {exc}")
        finally:
            close_client(client)
        time.sleep(5)
    if last_results:
        print("post-restore baseline remained unhealthy after timeout")
        print_post_restore_results(last_results)
        raise RuntimeError("post-restore baseline did not recover to a healthy state")
    raise RuntimeError("unable to collect post-restore status")


def main() -> int:
    host = os.environ["PRINTER_HOST"]
    user = os.environ["PRINTER_USER"]
    password = os.environ["PRINTER_PASSWORD"]

    client = connect_ssh_with_retry(host, user, password, 60)
    sftp = client.open_sftp()

    try:
        sftp.put(str(LOCAL_CANDIDATE), REMOTE_GKLIB_SWAP)
        sftp.put(str(LOCAL_HELPER), REMOTE_HELPER_SWAP)
        must_run(client, f"chmod 755 {REMOTE_GKLIB_SWAP} {REMOTE_HELPER_SWAP}")

        must_run(
            client,
            f"rm -f {REMOTE_GKLIB_BACKUP} {REMOTE_HELPER_BACKUP} && "
            f"cp {REMOTE_GKLIB} {REMOTE_GKLIB_BACKUP} && cp {REMOTE_HELPER} {REMOTE_HELPER_BACKUP} && "
            f"cp {REMOTE_GKLIB_SWAP} {REMOTE_GKLIB} && cp {REMOTE_HELPER_SWAP} {REMOTE_HELPER}",
        )
        must_run(client, f"chmod 755 {REMOTE_GKLIB} {REMOTE_HELPER}")
        must_run(client, f"mkdir -p /tmp/rinkhals && : > {RINKHALS_GKLIB_LOG}")

        for cmd in [RINKHALS_STOP, f"sh -lc '{RINKHALS_START}'"]:
            out, err, code = run(client, cmd, timeout=240)
            print(f">>> {cmd} [code={code}]")
            print(out[:12000].strip())
            if err.strip():
                print("stderr:")
                print(err[:4000].strip())
            time.sleep(5.0)

        time.sleep(25.0)
        for cmd in [
            socket_status_cmd("/tmp/unix_uds1"),
            "ps | grep './gklib -a /tmp/unix_uds1 rinkhals_gklib.cfg' | grep -v grep",
            f"tail -n 200 {RINKHALS_GKLIB_LOG}",
            f"grep -E 'Loaded MCU|Timeout on connect|Serial connection closed|Unable to obtain' {RINKHALS_GKLIB_LOG} | tail -n 80 || true",
        ]:
            out, err, code = run(client, cmd, timeout=120)
            print(f">>> {cmd} [code={code}]")
            print(out[:20000].strip())
            if err.strip():
                print("stderr:")
                print(err[:4000].strip())
    finally:
        try:
            must_run(
                client,
                f"if [ -f {REMOTE_GKLIB_BACKUP} ]; then cp {REMOTE_GKLIB_BACKUP} {REMOTE_GKLIB}; fi && "
                f"if [ -f {REMOTE_HELPER_BACKUP} ]; then cp {REMOTE_HELPER_BACKUP} {REMOTE_HELPER}; fi && "
                f"chmod 755 {REMOTE_GKLIB} {REMOTE_HELPER}",
            )
        except Exception as exc:
            print(f"restore_copy_error: {exc}")
        send_reboot(client, "reboot_send_error")
        close_client(client)
        wait_for_post_restore_health(host, user, password, 420)
        try:
            sftp.close()
        except Exception:
            pass
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
