#!/usr/bin/env python3

from __future__ import annotations

import argparse
import itertools
import pathlib
import re
import shlex
import time
from dataclasses import dataclass

import paramiko


REMOTE_DIR = "/userdata/app/gk"
REMOTE_CFG_LINK = f"{REMOTE_DIR}/rinkhals_gklib.cfg"
REMOTE_TEMP_CFG = f"{REMOTE_DIR}/printer_data/config/printer.generated.nozzle_shim.cfg"
REMOTE_SHIM = f"{REMOTE_DIR}/serial_shim"
REMOTE_CANDIDATE_DIR = f"{REMOTE_DIR}/nozzle_compare_candidate"
REMOTE_CANDIDATE = f"{REMOTE_CANDIDATE_DIR}/gklib_candidate"
REMOTE_CANDIDATE_HELPER = f"{REMOTE_CANDIDATE_DIR}/libc_helper.so"
REMOTE_RESET = f"{REMOTE_DIR}/mcu_reset"
DEFAULT_SHIM_LOG_PORT = 9105
DEFAULT_BAUD = 576000
DEFAULT_MCU_SECTION = "nozzle_mcu"
DEFAULT_SERIAL_DEVICE = "/dev/ttyS5"
DEFAULT_RESET_PRINTER = "ks1"
DEFAULT_RESET_SETTLE_SECONDS = 5.0
DEFAULT_CASE_ORDER = "original-first"


@dataclass
class RunArtifacts:
    name: str
    shim_log: pathlib.Path
    gklib_out: pathlib.Path


@dataclass
class ResetArtifacts:
    name: str
    log: pathlib.Path


class RemoteRunner:
    def __init__(self, host: str, user: str, password: str):
        self._client = paramiko.SSHClient()
        self._client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        self._client.connect(
            host,
            username=user,
            password=password,
            timeout=10,
            banner_timeout=10,
            auth_timeout=10,
        )
        self._sftp = self._client.open_sftp()

    def close(self) -> None:
        self._client.close()

    def run(self, cmd: str, timeout: int = 30) -> tuple[str, str, int]:
        stdin, stdout, stderr = self._client.exec_command(cmd, timeout=timeout)
        out = stdout.read().decode("utf-8", "replace")
        err = stderr.read().decode("utf-8", "replace")
        code = stdout.channel.recv_exit_status()
        return out, err, code

    def must_run(self, cmd: str, timeout: int = 30) -> str:
        out, err, code = self.run(cmd, timeout=timeout)
        if code != 0:
            raise RuntimeError(
                f"command failed ({code}): {cmd}\nSTDOUT:\n{out}\nSTDERR:\n{err}"
            )
        return out

    def upload(self, local_path: pathlib.Path, remote_path: str, mode: int = 0o755) -> None:
        self._sftp.put(str(local_path), remote_path)
        self.must_run(f"chmod {mode:o} {remote_path}")

    def fetch(self, remote_path: str, local_path: pathlib.Path) -> None:
        self._sftp.get(remote_path, str(local_path))

    def read_text(self, remote_path: str) -> str:
        with self._sftp.file(remote_path, "r") as handle:
            return handle.read().decode("utf-8", "replace")

    def write_text(self, remote_path: str, content: str, mode: int = 0o644) -> None:
        with self._sftp.file(remote_path, "w") as handle:
            handle.write(content.encode("utf-8"))
        self.must_run(f"chmod {mode:o} {remote_path}")


def rewrite_mcu_serial(config_text: str, mcu_section: str, listen_port: int) -> str:
    lines = config_text.splitlines(True)
    output: list[str] = []
    in_nozzle_section = False
    replaced = False
    replacement = f"serial : tcp@127.0.0.1:{listen_port}\n"
    target_header = f"[mcu {mcu_section}]".lower() if mcu_section.lower() != "mcu" else "[mcu]"
    for line in lines:
        stripped = line.strip()
        if stripped.startswith("[") and stripped.endswith("]"):
            in_nozzle_section = stripped.lower() == target_header
            output.append(line)
            continue
        if in_nozzle_section and stripped.lower().startswith("serial"):
            output.append(replacement)
            replaced = True
            continue
        output.append(line)
    if not replaced:
        raise RuntimeError(f"failed to replace {target_header} serial line")
    return "".join(output)


def parse_frames(text: str) -> list[tuple[str, int, str]]:
    frames: list[tuple[str, int, str]] = []
    for line in text.splitlines():
        match = re.search(r"dir=([^ ]+) bytes=(\d+) hex=([0-9a-f]+)", line)
        if match:
            frames.append((match.group(1), int(match.group(2)), match.group(3)))
    return frames


def first_difference(seq_a: list[tuple[str, int, str]], seq_b: list[tuple[str, int, str]]):
    for idx, pair in enumerate(itertools.zip_longest(seq_a, seq_b, fillvalue=None)):
        if pair[0] != pair[1]:
            return idx, pair[0], pair[1]
    return None


def format_difference(label: str, seq_a: list[tuple[str, int, str]], seq_b: list[tuple[str, int, str]]) -> list[str]:
    diff = first_difference(seq_a, seq_b)
    lines = [f"{label} original frame count: {len(seq_a)}", f"{label} candidate frame count: {len(seq_b)}"]
    if diff is None:
        lines.append(f"{label} first difference: none")
    else:
        idx, original_frame, candidate_frame = diff
        lines.append(f"{label} first difference index: {idx}")
        lines.append(f"{label} original frame: {original_frame}")
        lines.append(f"{label} candidate frame: {candidate_frame}")
    return lines


def extract_timeout_targets(text: str) -> list[str]:
    return re.findall(r"mcu '([^']+)': Timeout on connect", text)


def start_background(remote: RemoteRunner, cmd: str, timeout: int = 30) -> int:
    out = remote.must_run(cmd, timeout=timeout).strip().splitlines()
    if not out:
        raise RuntimeError(f"no pid returned for background command: {cmd}")
    return int(out[-1].strip())


def stop_live_service(remote: RemoteRunner) -> None:
    remote.run(
        "killall gklib 2>/dev/null || true; "
        "pkill -f '/userdata/app/gk/gklib_candidate' 2>/dev/null || true; "
        "pkill -f '/userdata/app/gk/nozzle_compare_candidate/gklib_candidate' 2>/dev/null || true; "
        "killall serial_shim 2>/dev/null || true"
    )


def start_live_service(remote: RemoteRunner) -> int:
    cmd = (
        f"sh -lc 'cd {REMOTE_DIR} && export LD_LIBRARY_PATH={REMOTE_DIR}:$LD_LIBRARY_PATH; "
        "nohup ./gklib -a /tmp/unix_uds1 rinkhals_gklib.cfg >/tmp/gklib.restore.log 2>&1 & echo $!'"
    )
    return start_background(remote, cmd)


def reset_mcu(
    remote: RemoteRunner,
    name: str,
    artifacts_dir: pathlib.Path,
    remote_reset_path: str,
    printer: str,
    pre_delay: str,
    hold_duration: str,
    settle_seconds: float,
) -> ResetArtifacts:
    cmd = (
        "sh -lc "
        + shlex.quote(
            f"cd {REMOTE_DIR} && {remote_reset_path} --printer {shlex.quote(printer)} "
            f"--pre-delay {shlex.quote(pre_delay)} --hold {shlex.quote(hold_duration)}"
        )
    )
    out, err, code = remote.run(cmd, timeout=30)
    log_path = artifacts_dir / f"{name}.reset.log"
    log_path.write_text(
        f"command: {cmd}\nexit_code: {code}\nstdout:\n{out}\nstderr:\n{err}\n"
    )
    if code != 0:
        raise RuntimeError(
            f"mcu reset failed for {name} ({code})\nSTDOUT:\n{out}\nSTDERR:\n{err}"
        )
    time.sleep(settle_seconds)
    return ResetArtifacts(name=name, log=log_path)


def run_case(
    remote: RemoteRunner,
    name: str,
    binary_path: str,
    ld_library_path: str,
    artifacts_dir: pathlib.Path,
    listen_port: int,
    baud: int,
    serial_device: str,
    duration_seconds: float,
) -> RunArtifacts:
    shim_log = f"/tmp/nozzle_{name}.shim.log"
    shim_stdout = f"/tmp/nozzle_{name}.shim.stdout"
    gklib_out = f"/tmp/nozzle_{name}.gklib.out"
    uds_path = f"/tmp/unix_uds_test_{name}"
    remote.run(f"rm -f {shim_log} {shim_stdout} {gklib_out} {uds_path}")
    shim_cmd = (
        f"sh -lc 'cd {REMOTE_DIR} && export LD_LIBRARY_PATH={REMOTE_DIR}:$LD_LIBRARY_PATH; "
        f"nohup {REMOTE_SHIM} -listen 127.0.0.1:{listen_port} -serial {serial_device} -baud {baud} -log {shim_log} >{shim_stdout} 2>&1 & echo $!'"
    )
    shim_pid = start_background(remote, shim_cmd)
    time.sleep(1.0)
    gklib_cmd = (
        f"sh -lc 'cd {REMOTE_DIR} && export LD_LIBRARY_PATH={ld_library_path}:$LD_LIBRARY_PATH; "
        f"nohup {binary_path} -a {uds_path} {REMOTE_TEMP_CFG} >{gklib_out} 2>&1 & echo $!'"
    )
    gklib_pid = start_background(remote, gklib_cmd)
    time.sleep(duration_seconds)
    remote.run(f"kill {gklib_pid} {shim_pid} 2>/dev/null || true")
    time.sleep(1.0)

    local_shim_log = artifacts_dir / f"{name}.shim.log"
    local_gklib_out = artifacts_dir / f"{name}.gklib.out"
    remote.fetch(shim_log, local_shim_log)
    remote.fetch(gklib_out, local_gklib_out)
    return RunArtifacts(name=name, shim_log=local_shim_log, gklib_out=local_gklib_out)


def build_summary(
    original: RunArtifacts,
    candidate: RunArtifacts,
    restored_pid: int,
    restored_ps: str,
    case_order: str,
    reset_enabled: bool,
    reset_printer: str,
    reset_pre_delay: str,
    reset_hold: str,
    reset_settle: float,
) -> str:
    summary_lines: list[str] = []
    summary_lines.append(f"case order: {case_order}")
    summary_lines.append(f"mcu reset enabled: {reset_enabled}")
    if not reset_enabled:
        summary_lines.append(
            "warning: skip-reset runs are sequential and asymmetric; the second case observes nozzle_mcu state left by the first case"
        )
    if reset_enabled:
        summary_lines.append(f"mcu reset printer: {reset_printer}")
        summary_lines.append(f"mcu reset pre-delay: {reset_pre_delay}")
        summary_lines.append(f"mcu reset hold: {reset_hold}")
        summary_lines.append(f"mcu reset settle seconds: {reset_settle}")
    original_frames = parse_frames(original.shim_log.read_text())
    candidate_frames = parse_frames(candidate.shim_log.read_text())
    summary_lines.extend(format_difference("all frames", original_frames, candidate_frames))
    summary_lines.extend(
        format_difference(
            "tcp->serial",
            [frame for frame in original_frames if frame[0] == "tcp->serial"],
            [frame for frame in candidate_frames if frame[0] == "tcp->serial"],
        )
    )
    summary_lines.extend(
        format_difference(
            "serial->tcp",
            [frame for frame in original_frames if frame[0] == "serial->tcp"],
            [frame for frame in candidate_frames if frame[0] == "serial->tcp"],
        )
    )

    original_out = original.gklib_out.read_text()
    candidate_out = candidate.gklib_out.read_text()
    needles = [
        "Timeout on connect",
        "Serial connection closed",
        "Identify Request: identify offset=0 count=40",
    ]
    for needle in needles:
        summary_lines.append(f"original contains {needle!r}: {needle in original_out}")
        summary_lines.append(f"candidate contains {needle!r}: {needle in candidate_out}")
    summary_lines.append(f"original timeout targets: {extract_timeout_targets(original_out)}")
    summary_lines.append(f"candidate timeout targets: {extract_timeout_targets(candidate_out)}")

    summary_lines.append(f"restored live gklib pid: {restored_pid}")
    summary_lines.append("live process after restore:")
    summary_lines.append(restored_ps.strip())
    return "\n".join(summary_lines) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description="Compare original and candidate gklib MCU serial traffic through a TCP shim.")
    parser.add_argument("--host", required=True)
    parser.add_argument("--user", default="root")
    parser.add_argument("--password", required=True)
    parser.add_argument("--local-shim", default="build/serial_shim_arm")
    parser.add_argument("--local-candidate", default="gklib_uclibc_current")
    parser.add_argument("--local-helper", default="internal/pkg/chelper/libc_helper.so")
    parser.add_argument("--artifacts-dir", default="build/nozzle_compare")
    parser.add_argument("--listen-port", type=int, default=DEFAULT_SHIM_LOG_PORT)
    parser.add_argument("--baud", type=int, default=DEFAULT_BAUD)
    parser.add_argument("--mcu-section", default=DEFAULT_MCU_SECTION)
    parser.add_argument("--serial-device", default=DEFAULT_SERIAL_DEVICE)
    parser.add_argument("--duration", type=float, default=18.0)
    parser.add_argument("--local-reset", default="build/mcu_reset_arm")
    parser.add_argument("--remote-reset", default=REMOTE_RESET)
    parser.add_argument("--reset-printer", default=DEFAULT_RESET_PRINTER)
    parser.add_argument("--reset-pre-delay", default="1s")
    parser.add_argument("--reset-hold", default="1s")
    parser.add_argument("--reset-settle", type=float, default=DEFAULT_RESET_SETTLE_SECONDS)
    parser.add_argument("--skip-reset", action="store_true")
    parser.add_argument(
        "--case-order",
        choices=["original-first", "candidate-first"],
        default=DEFAULT_CASE_ORDER,
        help="Execution order for sequential runs. In --skip-reset mode, the second case inherits MCU state from the first.",
    )
    args = parser.parse_args()

    local_shim = pathlib.Path(args.local_shim)
    local_candidate = pathlib.Path(args.local_candidate)
    local_helper = pathlib.Path(args.local_helper)
    local_reset = pathlib.Path(args.local_reset)
    artifacts_dir = pathlib.Path(args.artifacts_dir)
    artifacts_dir.mkdir(parents=True, exist_ok=True)
    if not local_shim.exists():
        raise SystemExit(f"missing local shim: {local_shim}")
    if not local_candidate.exists():
        raise SystemExit(f"missing local candidate binary: {local_candidate}")
    if not local_helper.exists():
        raise SystemExit(f"missing local helper library: {local_helper}")
    if not args.skip_reset and not local_reset.exists():
        raise SystemExit(f"missing local reset binary: {local_reset}")

    remote = RemoteRunner(args.host, args.user, args.password)
    original: RunArtifacts | None = None
    candidate: RunArtifacts | None = None
    case_plan = [
        ("original", f"{REMOTE_DIR}/gklib", REMOTE_DIR),
        ("candidate", REMOTE_CANDIDATE, f"{REMOTE_CANDIDATE_DIR}:{REMOTE_DIR}"),
    ]
    if args.case_order == "candidate-first":
        case_plan.reverse()
    try:
        remote.upload(local_shim, REMOTE_SHIM)
        remote.must_run(f"mkdir -p {REMOTE_CANDIDATE_DIR}")
        remote.upload(local_candidate, REMOTE_CANDIDATE)
        remote.upload(local_helper, REMOTE_CANDIDATE_HELPER)
        if not args.skip_reset:
            remote.upload(local_reset, args.remote_reset)
        target_cfg = remote.must_run(f"readlink -f {REMOTE_CFG_LINK}").strip()
        config_text = remote.read_text(target_cfg)
        remote.write_text(REMOTE_TEMP_CFG, rewrite_mcu_serial(config_text, args.mcu_section, args.listen_port))

        stop_live_service(remote)
        time.sleep(2.0)

        for name, binary_path, ld_library_path in case_plan:
            if not args.skip_reset:
                reset_mcu(
                    remote,
                    name,
                    artifacts_dir,
                    args.remote_reset,
                    args.reset_printer,
                    args.reset_pre_delay,
                    args.reset_hold,
                    args.reset_settle,
                )

            artifacts = run_case(
                remote,
                name,
                binary_path,
                ld_library_path,
                artifacts_dir,
                args.listen_port,
                args.baud,
                args.serial_device,
                args.duration,
            )
            if name == "original":
                original = artifacts
            else:
                candidate = artifacts
    finally:
        stop_live_service(remote)
        time.sleep(1.0)
        restored_pid = start_live_service(remote)
        time.sleep(8.0)
        restored_ps = remote.must_run(
            "ps | grep './gklib -a /tmp/unix_uds1 rinkhals_gklib.cfg' | grep -v grep",
            timeout=20,
        )
        if original is None or candidate is None:
            summary = (
                "comparison did not complete successfully\n"
                f"restored live gklib pid: {restored_pid}\n"
                "live process after restore:\n"
                f"{restored_ps.strip()}\n"
            )
        else:
            summary = build_summary(
                original,
                candidate,
                restored_pid,
                restored_ps,
                args.case_order,
                not args.skip_reset,
                args.reset_printer,
                args.reset_pre_delay,
                args.reset_hold,
                args.reset_settle,
            )
        summary_path = artifacts_dir / "summary.txt"
        summary_path.write_text(summary)
        print(summary, end="")
        remote.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
