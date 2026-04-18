#!/usr/bin/env python3

from __future__ import annotations

import argparse
import os
import re
import time
from pathlib import Path

import paramiko

REMOTE_DIR = "/userdata/app/gk"
REMOTE_CFG = "rinkhals_gklib.cfg"
REMOTE_CANDIDATE_DIR = f"{REMOTE_DIR}/nozzle_compare_candidate"
REMOTE_CANDIDATE = f"{REMOTE_CANDIDATE_DIR}/gklib_candidate"
REMOTE_HELPER = f"{REMOTE_CANDIDATE_DIR}/libc_helper.so"
LOCAL_CANDIDATE = Path("gklib_uclibc_current")
LOCAL_HELPER = Path("internal/pkg/chelper/libc_helper.so")
TEST_LOG = "/tmp/gklib_candidate_direct.log"
TEST_UDS = "/tmp/unix_uds_candidate_direct"
ORIGINAL_LOG = "/tmp/gklib_original_direct.log"


def run(client: paramiko.SSHClient, cmd: str, timeout: int = 30) -> tuple[str, str, int]:
    stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode("utf-8", "replace")
    err = stderr.read().decode("utf-8", "replace")
    code = stdout.channel.recv_exit_status()
    return out, err, code


def must_run(client: paramiko.SSHClient, cmd: str, timeout: int = 30) -> str:
    out, err, code = run(client, cmd, timeout=timeout)
    if code != 0:
        raise RuntimeError(
            f"command failed ({code}): {cmd}\nSTDOUT:\n{out}\nSTDERR:\n{err}"
        )
    return out


def start_bg(client: paramiko.SSHClient, cmd: str, timeout: int = 30) -> int:
    out = must_run(client, cmd, timeout=timeout).strip().splitlines()
    if not out:
        raise RuntimeError(f"no pid returned for {cmd}")
    return int(out[-1].strip())


def list_matching_pids(client: paramiko.SSHClient, pattern: str) -> list[str]:
    out, _, code = run(client, f"ps | grep '{pattern}' | grep -v grep", timeout=20)
    if code != 0:
        return []
    pids: list[str] = []
    for line in out.splitlines():
        match = re.match(r"\s*(\d+)", line)
        if match:
            pids.append(match.group(1))
    return pids


def kill_matching_processes(client: paramiko.SSHClient, pattern: str) -> None:
    pids = list_matching_pids(client, pattern)
    if not pids:
        return
    run(client, "kill " + " ".join(pids), timeout=20)
    time.sleep(2.0)
    pids = list_matching_pids(client, pattern)
    if not pids:
        return
    run(client, "kill -9 " + " ".join(pids), timeout=20)
    time.sleep(2.0)


def upload_with_retry(
    client: paramiko.SSHClient,
    sftp: paramiko.SFTPClient,
    local_path: Path,
    remote_path: str,
) -> None:
    try:
        sftp.put(str(local_path), remote_path)
    except OSError:
        run(client, f"rm -f {remote_path}")
        sftp.put(str(local_path), remote_path)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=["candidate", "original"], default="candidate")
    parser.add_argument("--settle-after-stop", type=float, default=2.0)
    args = parser.parse_args()

    host = os.environ["PRINTER_HOST"]
    user = os.environ["PRINTER_USER"]
    password = os.environ["PRINTER_PASSWORD"]

    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(
        host,
        username=user,
        password=password,
        timeout=10,
        banner_timeout=10,
        auth_timeout=10,
    )
    sftp = client.open_sftp()
    vendor_pattern = "./gklib -a /tmp/unix_uds1 rinkhals_gklib.cfg"
    if args.mode == "candidate":
        mode_binary = REMOTE_CANDIDATE
        mode_ld_path = f"{REMOTE_CANDIDATE_DIR}:{REMOTE_DIR}"
        mode_pattern = f"{REMOTE_CANDIDATE} -a {TEST_UDS} {REMOTE_CFG}"
        mode_log = TEST_LOG
    else:
        mode_binary = "./gklib"
        mode_ld_path = REMOTE_DIR
        mode_pattern = f"./gklib -a {TEST_UDS} {REMOTE_CFG}"
        mode_log = ORIGINAL_LOG

    try:
        if args.mode == "candidate":
            must_run(client, f"mkdir -p {REMOTE_CANDIDATE_DIR}")
            upload_with_retry(client, sftp, LOCAL_CANDIDATE, REMOTE_CANDIDATE)
            upload_with_retry(client, sftp, LOCAL_HELPER, REMOTE_HELPER)
            must_run(client, f"chmod 755 {REMOTE_CANDIDATE} {REMOTE_HELPER}")

        kill_matching_processes(client, vendor_pattern)
        kill_matching_processes(client, f"{REMOTE_CANDIDATE} -a {TEST_UDS} {REMOTE_CFG}")
        time.sleep(args.settle_after_stop)
        run(client, f"rm -f {TEST_LOG} {ORIGINAL_LOG} {TEST_UDS}")

        candidate_pid = start_bg(
            client,
            "sh -lc 'cd {remote_dir} && export USE_MUTABLE_CONFIG=1; export LD_LIBRARY_PATH={ld_path}:$LD_LIBRARY_PATH; "
            "nohup nice -n -20 {binary} -a {uds} {cfg} >{log} 2>&1 & echo $!'".format(
                remote_dir=REMOTE_DIR,
                ld_path=mode_ld_path,
                binary=mode_binary,
                uds=TEST_UDS,
                cfg=REMOTE_CFG,
                log=mode_log,
            ),
            timeout=30,
        )
        time.sleep(18.0)

        try:
            log_text = sftp.file(mode_log, "r").read().decode("utf-8", "replace")
        except Exception as exc:  # pragma: no cover - diagnostic path
            log_text = f"<unable to read log: {exc}>"
        ps_out, ps_err, ps_code = run(
            client,
            f"ps | grep '{mode_pattern}' | grep -v grep",
            timeout=20,
        )
        _, _, uds_code = run(client, f"test -S {TEST_UDS}")

        print(f"mode: {args.mode}")
        print(f"pid: {candidate_pid}")
        print(f"process_running: {ps_code == 0}")
        print(f"socket_present: {uds_code == 0}")
        if ps_out.strip():
            print("process:")
            print(ps_out.strip())
        if ps_err.strip():
            print("process_stderr:")
            print(ps_err.strip())
        print("log_begin")
        print("\n".join(log_text.splitlines()[:260]))
        print("log_end")
    finally:
        kill_matching_processes(client, mode_pattern)
        kill_matching_processes(client, vendor_pattern)
        restored_pid = start_bg(
            client,
            f"sh -lc 'cd {REMOTE_DIR} && export USE_MUTABLE_CONFIG=1; export LD_LIBRARY_PATH={REMOTE_DIR}:$LD_LIBRARY_PATH; nohup nice -n -20 ./gklib -a /tmp/unix_uds1 {REMOTE_CFG} >/tmp/gklib.restore.log 2>&1 & echo $!'",
            timeout=30,
        )
        time.sleep(8.0)
        restored_ps = must_run(
            client,
            "ps | grep './gklib -a /tmp/unix_uds1 rinkhals_gklib.cfg' | grep -v grep",
            timeout=20,
        )
        print(f"restored_pid: {restored_pid}")
        print("restored_process:")
        print(restored_ps.strip())
        sftp.close()
        client.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
