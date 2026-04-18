#!/usr/bin/env python3
"""Compare TMC UART serial traffic between gklib_real (manufacturer) and our candidate.

Runs each binary with serial_shim proxying /dev/ttyS3, captures all frames,
then extracts and compares the tmcuart_send command payloads byte-for-byte.
"""
from __future__ import annotations

import argparse
import itertools
import os
import pathlib
import re
import shlex
import sys
import time
from dataclasses import dataclass, field

import paramiko

REMOTE_DIR = "/userdata/app/gk"
REMOTE_SHIM = f"{REMOTE_DIR}/serial_shim"
REMOTE_MCU_RESET = f"{REMOTE_DIR}/mcu_reset"
REMOTE_ORIGINAL = f"{REMOTE_DIR}/gklib_real"
REMOTE_CANDIDATE = f"{REMOTE_DIR}/gklib"
REMOTE_CFG_LINK = f"{REMOTE_DIR}/rinkhals_gklib.cfg"
REMOTE_TEMP_CFG = f"{REMOTE_DIR}/printer_data/config/printer.generated.tmc_shim.cfg"

DEFAULT_LISTEN_PORT = 9108
DEFAULT_BAUD = 576000
DEFAULT_SERIAL = "/dev/ttyS3"
DEFAULT_MCU_SECTION = "mcu"
DEFAULT_DURATION = 120.0
DEFAULT_RESET_PRINTER = "ks1"


class RemoteRunner:
    def __init__(self, host: str, user: str, password: str):
        self._client = paramiko.SSHClient()
        self._client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        self._client.connect(host, username=user, password=password,
                             timeout=10, banner_timeout=10, auth_timeout=10)
        self._sftp = self._client.open_sftp()

    def close(self):
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
            raise RuntimeError(f"command failed ({code}): {cmd}\nSTDOUT:\n{out}\nSTDERR:\n{err}")
        return out

    def read_text(self, remote_path: str) -> str:
        with self._sftp.file(remote_path, "r") as f:
            return f.read().decode("utf-8", "replace")

    def write_text(self, remote_path: str, content: str, mode: int = 0o644):
        with self._sftp.file(remote_path, "w") as f:
            f.write(content.encode("utf-8"))
        self.must_run(f"chmod {mode:o} {remote_path}")

    def fetch(self, remote_path: str, local_path: pathlib.Path):
        self._sftp.get(remote_path, str(local_path))


def rewrite_mcu_serial(config_text: str, mcu_section: str, listen_port: int) -> str:
    """Replace the serial line in the given MCU section to point at the shim TCP port."""
    lines = config_text.splitlines(True)
    output: list[str] = []
    in_target = False
    replaced = False
    target_header = f"[mcu {mcu_section}]".lower() if mcu_section.lower() != "mcu" else "[mcu]"
    for line in lines:
        stripped = line.strip()
        if stripped.startswith("[") and stripped.endswith("]"):
            in_target = stripped.lower() == target_header
            output.append(line)
            continue
        if in_target and stripped.lower().startswith("serial"):
            output.append(f"serial: tcp@127.0.0.1:{listen_port}\n")
            replaced = True
            continue
        output.append(line)
    if not replaced:
        raise RuntimeError(f"failed to replace serial in {target_header}")
    return "".join(output)


@dataclass
class Frame:
    timestamp: str
    session: int
    direction: str
    size: int
    hexdata: str


def parse_shim_log(text: str) -> list[Frame]:
    frames: list[Frame] = []
    for line in text.splitlines():
        m = re.match(
            r"(\S+ \S+) session=(\d+) dir=(\S+) bytes=(\d+) hex=([0-9a-fA-F]+)",
            line,
        )
        if m:
            frames.append(Frame(
                timestamp=m.group(1),
                session=int(m.group(2)),
                direction=m.group(3),
                size=int(m.group(4)),
                hexdata=m.group(5),
            ))
    return frames


def decode_klipper_frames(hexdata: str) -> list[bytes]:
    """Split raw hex into individual Klipper protocol frames (0x7e delimited)."""
    raw = bytes.fromhex(hexdata)
    # Klipper frames: length byte, content, CRC, 0x7e sync
    # Split on 0x7e sync byte
    frames: list[bytes] = []
    buf = bytearray()
    for b in raw:
        if b == 0x7e:
            if buf:
                frames.append(bytes(buf))
            buf = bytearray()
        else:
            buf.append(b)
    if buf:
        frames.append(bytes(buf))
    return frames


def extract_klipper_payload(frame_bytes: bytes) -> bytes:
    """Return just the payload (strip length byte and trailing CRC)."""
    if len(frame_bytes) < 3:
        return frame_bytes
    # frame_bytes[0] = length, frame_bytes[1:-2] = seq+payload, frame_bytes[-2:] = CRC
    return frame_bytes[1:-2]


def stop_all(remote: RemoteRunner):
    remote.run(
        "killall gklib gklib_real 2>/dev/null || true; "
        "pkill -f gklib_candidate 2>/dev/null || true; "
        "killall serial_shim 2>/dev/null || true",
        timeout=10,
    )
    time.sleep(1.0)


def firmware_restart_via_api(remote: RemoteRunner, uds_path: str = "/tmp/unix_uds1",
                             timeout: int = 10):
    """Send FIRMWARE_RESTART through the live gklib API to reset MCU to identify mode.

    Returns quickly after sending the command; caller should kill gklib immediately
    to prevent it from reconnecting to MCUs before the shim starts.
    """
    print("  sending FIRMWARE_RESTART via gklib API...")
    script = (
        "import json, socket, sys\n"
        "payload = json.dumps({'id': 99, 'method': 'gcode/script', "
        "'params': {'script': 'FIRMWARE_RESTART'}}).encode() + b'\\x03'\n"
        "sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)\n"
        f"sock.settimeout({timeout})\n"
        f"sock.connect('{uds_path}')\n"
        "sock.sendall(payload)\n"
        "try:\n"
        "    data = sock.recv(4096)\n"
        "    print(data.decode('utf-8', 'replace'))\n"
        "except socket.timeout:\n"
        "    print('timeout waiting for response (expected after FIRMWARE_RESTART)')\n"
        "except Exception as e:\n"
        "    print(f'recv error: {e}')\n"
        "sock.close()\n"
    )
    out, err, code = remote.run(f"python3 -c {shlex.quote(script)}", timeout=timeout + 5)
    print(f"  FIRMWARE_RESTART result: exit={code}")
    if out.strip():
        print(f"  response: {out.strip()[:200]}")


def wait_for_gklib_ready(remote: RemoteRunner, timeout: int = 90):
    """Wait for the boot gklib to reach Ready state."""
    print(f"  waiting for gklib to reach Ready (up to {timeout}s)...")
    deadline = time.time() + timeout
    while time.time() < deadline:
        out, _, code = remote.run(
            "ps | grep './gklib' | grep -v grep | head -1", timeout=5
        )
        if code != 0 or not out.strip():
            time.sleep(3)
            continue
        # Check if we can query the state
        script = (
            "import json, socket\n"
            "payload = json.dumps({'id': 1, 'method': 'info', 'params': {}}).encode() + b'\\x03'\n"
            "sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)\n"
            "sock.settimeout(5)\n"
            "try:\n"
            "    sock.connect('/tmp/unix_uds1')\n"
            "    sock.sendall(payload)\n"
            "    data = b''\n"
            "    while True:\n"
            "        chunk = sock.recv(4096)\n"
            "        if not chunk: break\n"
            "        data += chunk\n"
            "        if b'\\x03' in data:\n"
            "            data = data.split(b'\\x03', 1)[0]\n"
            "            break\n"
            "    print(data.decode('utf-8', 'replace'))\n"
            "except Exception as e:\n"
            "    print(f'ERR:{e}')\n"
            "finally:\n"
            "    sock.close()\n"
        )
        out, _, _ = remote.run(f"python3 -c {shlex.quote(script)}", timeout=10)
        if '"state":"ready"' in out.lower() or '"state": "ready"' in out.lower():
            print("  gklib is Ready")
            return True
        time.sleep(3)
    print("  WARNING: gklib did not reach Ready within timeout")
    return False


def wait_for_printer(host: str, user: str, password: str, timeout: int = 180) -> RemoteRunner:
    """Wait for the printer to come back online after a power cycle."""
    print(f"  waiting for printer at {host} (up to {timeout}s)...")
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            remote = RemoteRunner(host, user, password)
            # Verify it's actually up
            remote.must_run("uptime", timeout=5)
            uptime_out, _, _ = remote.run("uptime")
            print(f"  printer online: {uptime_out.strip()}")
            return remote
        except Exception:
            time.sleep(3)
    raise RuntimeError(f"printer at {host} did not come back within {timeout}s")


def reset_mcu(remote: RemoteRunner, printer: str = DEFAULT_RESET_PRINTER):
    print(f"  resetting MCU (printer={printer})...")
    out, err, code = remote.run(
        f"sh -lc '{REMOTE_MCU_RESET} --printer {shlex.quote(printer)} --pre-delay 1s --hold 1s --cleanup'",
        timeout=15,
    )
    if code != 0:
        print(f"  WARNING: mcu_reset exit {code}: {err.strip()}", file=sys.stderr)
    time.sleep(5.0)


def start_background(remote: RemoteRunner, cmd: str) -> int:
    out = remote.must_run(cmd, timeout=15).strip().splitlines()
    if not out:
        raise RuntimeError(f"no PID from: {cmd}")
    return int(out[-1].strip())


def run_case(
    remote: RemoteRunner,
    name: str,
    binary: str,
    listen_port: int,
    baud: int,
    serial_device: str,
    duration: float,
) -> tuple[str, str]:
    """Run one case: start shim, start gklib, wait, kill, return (shim_log_text, gklib_out_text)."""
    shim_log = f"/tmp/tmc_compare_{name}.shim.log"
    gklib_out = f"/tmp/tmc_compare_{name}.gklib.out"
    uds_path = f"/tmp/unix_uds_tmc_{name}"

    remote.run(f"rm -f {shim_log} {gklib_out} {uds_path}")

    # Start shim
    shim_cmd = (
        f"sh -lc 'cd {REMOTE_DIR} && export LD_LIBRARY_PATH={REMOTE_DIR}:$LD_LIBRARY_PATH; "
        f"nohup {REMOTE_SHIM} -listen 127.0.0.1:{listen_port} -serial {serial_device} "
        f"-baud {baud} -log {shim_log} >/tmp/tmc_compare_{name}.shim.stdout 2>&1 & echo $!'"
    )
    shim_pid = start_background(remote, shim_cmd)
    print(f"  shim PID={shim_pid}")
    time.sleep(1.0)

    # Start gklib
    gklib_cmd = (
        f"sh -lc 'cd {REMOTE_DIR} && export LD_LIBRARY_PATH={REMOTE_DIR}:$LD_LIBRARY_PATH; "
        f"nohup {binary} -a {uds_path} {REMOTE_TEMP_CFG} >{gklib_out} 2>&1 & echo $!'"
    )
    gklib_pid = start_background(remote, gklib_cmd)
    print(f"  gklib PID={gklib_pid} ({binary})")

    # Wait for it to connect and do TMC init, with progress monitoring
    print(f"  waiting {duration:.0f}s for TMC initialization...")
    check_interval = 15.0
    elapsed = 0.0
    while elapsed < duration:
        wait = min(check_interval, duration - elapsed)
        time.sleep(wait)
        elapsed += wait
        # Check progress: count TX and RX frames in shim log
        counts, _, _ = remote.run(
            f"grep -c 'tcp->serial' {shim_log} 2>/dev/null; "
            f"grep -c 'serial->tcp' {shim_log} 2>/dev/null",
            timeout=5,
        )
        parts = counts.strip().splitlines()
        tx = parts[0] if len(parts) > 0 else "?"
        rx = parts[1] if len(parts) > 1 else "?"
        # Check gklib for Ready state
        ready_out, _, _ = remote.run(
            f"grep -c 'project state: Ready' {gklib_out} 2>/dev/null || echo 0",
            timeout=5,
        )
        ready = ready_out.strip() != "0"
        status = "READY" if ready else "not ready"
        print(f"  [{elapsed:.0f}s/{duration:.0f}s] tx={tx} rx={rx} gklib={status}")
        if ready and elapsed >= 30:
            # gklib reached Ready; give it some more time for TMC init then stop
            print(f"  gklib reached Ready at {elapsed:.0f}s, waiting 30s more for TMC init...")
            time.sleep(30.0)
            break

    # Kill
    remote.run(f"kill {gklib_pid} {shim_pid} 2>/dev/null || true")
    time.sleep(2.0)
    # Force kill in case
    remote.run(f"kill -9 {gklib_pid} {shim_pid} 2>/dev/null || true")
    time.sleep(0.5)

    shim_text = ""
    gklib_text = ""
    try:
        shim_text, _, _ = remote.run(f"cat {shim_log}", timeout=10)
    except Exception as e:
        print(f"  WARNING: could not read shim log: {e}", file=sys.stderr)
    try:
        gklib_text, _, _ = remote.run(f"cat {gklib_out}", timeout=10)
    except Exception as e:
        print(f"  WARNING: could not read gklib output: {e}", file=sys.stderr)

    return shim_text, gklib_text


def compare_frames(
    original_frames: list[Frame],
    candidate_frames: list[Frame],
) -> list[str]:
    """Compare frames between original and candidate, focusing on tcp->serial (host to MCU)."""
    lines: list[str] = []

    orig_tx = [f for f in original_frames if f.direction == "tcp->serial"]
    cand_tx = [f for f in candidate_frames if f.direction == "tcp->serial"]
    orig_rx = [f for f in original_frames if f.direction == "serial->tcp"]
    cand_rx = [f for f in candidate_frames if f.direction == "serial->tcp"]

    lines.append(f"original: {len(orig_tx)} tx frames, {len(orig_rx)} rx frames")
    lines.append(f"candidate: {len(cand_tx)} tx frames, {len(cand_rx)} rx frames")
    lines.append("")

    # Decode all Klipper protocol frames from each side
    orig_klipper: list[bytes] = []
    for f in orig_tx:
        orig_klipper.extend(decode_klipper_frames(f.hexdata))
    cand_klipper: list[bytes] = []
    for f in cand_tx:
        cand_klipper.extend(decode_klipper_frames(f.hexdata))

    lines.append(f"original: {len(orig_klipper)} Klipper protocol frames (tx)")
    lines.append(f"candidate: {len(cand_klipper)} Klipper protocol frames (tx)")
    lines.append("")

    # Extract payloads and compare
    orig_payloads = [extract_klipper_payload(f) for f in orig_klipper]
    cand_payloads = [extract_klipper_payload(f) for f in cand_klipper]

    # Find first difference in tx payloads
    first_diff = None
    for i, (a, b) in enumerate(itertools.zip_longest(orig_payloads, cand_payloads)):
        if a != b:
            first_diff = i
            lines.append(f"FIRST TX DIFFERENCE at Klipper frame #{i}:")
            lines.append(f"  original:  {a.hex() if a else 'MISSING'}")
            lines.append(f"  candidate: {b.hex() if b else 'MISSING'}")
            break

    if first_diff is None:
        lines.append("ALL TX PAYLOADS IDENTICAL between original and candidate")
    else:
        # Count total differences
        diffs = 0
        for i, (a, b) in enumerate(itertools.zip_longest(orig_payloads, cand_payloads)):
            if a != b:
                diffs += 1
        lines.append(f"total tx payload differences: {diffs}")

    lines.append("")

    # Now compare RX (MCU responses) -- this tells us about bitbang quality
    orig_rx_klipper: list[bytes] = []
    for f in orig_rx:
        orig_rx_klipper.extend(decode_klipper_frames(f.hexdata))
    cand_rx_klipper: list[bytes] = []
    for f in cand_rx:
        cand_rx_klipper.extend(decode_klipper_frames(f.hexdata))

    lines.append(f"original: {len(orig_rx_klipper)} Klipper protocol frames (rx)")
    lines.append(f"candidate: {len(cand_rx_klipper)} Klipper protocol frames (rx)")

    # Show first few rx payloads from each for manual inspection
    lines.append("")
    lines.append("=== ORIGINAL TX payload samples (first 30) ===")
    for i, p in enumerate(orig_payloads[:30]):
        lines.append(f"  [{i:3d}] {p.hex()}")

    lines.append("")
    lines.append("=== CANDIDATE TX payload samples (first 30) ===")
    for i, p in enumerate(cand_payloads[:30]):
        lines.append(f"  [{i:3d}] {p.hex()}")

    lines.append("")
    lines.append("=== ORIGINAL RX payload samples (first 30) ===")
    orig_rx_payloads = [extract_klipper_payload(f) for f in orig_rx_klipper]
    for i, p in enumerate(orig_rx_payloads[:30]):
        lines.append(f"  [{i:3d}] {p.hex()}")

    lines.append("")
    lines.append("=== CANDIDATE RX payload samples (first 30) ===")
    cand_rx_payloads = [extract_klipper_payload(f) for f in cand_rx_klipper]
    for i, p in enumerate(cand_rx_payloads[:30]):
        lines.append(f"  [{i:3d}] {p.hex()}")

    # Byte-for-byte comparison of matching tx frames
    lines.append("")
    lines.append("=== BYTE-BY-BYTE TX COMPARISON (first 30 frames) ===")
    for i, (a, b) in enumerate(itertools.zip_longest(
        orig_payloads[:30], cand_payloads[:30]
    )):
        if a is None or b is None:
            lines.append(f"  [{i:3d}] {'MISSING':>8s} vs {'MISSING' if b is None else b.hex()}")
            continue
        if a == b:
            lines.append(f"  [{i:3d}] MATCH    {a.hex()}")
        else:
            lines.append(f"  [{i:3d}] DIFFER")
            lines.append(f"         orig: {a.hex()}")
            lines.append(f"         cand: {b.hex()}")
            # Show per-byte diff
            diff_positions = []
            for j in range(max(len(a), len(b))):
                ab = a[j] if j < len(a) else None
                bb = b[j] if j < len(b) else None
                if ab != bb:
                    diff_positions.append(j)
            lines.append(f"         diff at byte positions: {diff_positions}")

    return lines


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Compare TMC UART serial traffic between gklib_real and candidate"
    )
    parser.add_argument("--host", default=os.environ.get("PRINTER_HOST", "192.168.0.96"))
    parser.add_argument("--user", default=os.environ.get("PRINTER_USER", "root"))
    parser.add_argument("--password", default=os.environ.get("PRINTER_PASSWORD", "rockchip"))
    parser.add_argument("--listen-port", type=int, default=DEFAULT_LISTEN_PORT)
    parser.add_argument("--baud", type=int, default=DEFAULT_BAUD)
    parser.add_argument("--serial-device", default=DEFAULT_SERIAL)
    parser.add_argument("--mcu-section", default=DEFAULT_MCU_SECTION)
    parser.add_argument("--duration", type=float, default=DEFAULT_DURATION)
    parser.add_argument("--reset-printer", default=DEFAULT_RESET_PRINTER)
    parser.add_argument("--skip-reset", action="store_true",
                        help="Skip MCU reset between runs (not recommended)")
    parser.add_argument("--artifacts-dir", default="build/tmc_uart_compare")
    parser.add_argument("--original-first", action="store_true", default=True,
                        help="Run original binary first (default)")
    parser.add_argument("--candidate-first", action="store_true",
                        help="Run candidate binary first")
    parser.add_argument("--case", choices=["original", "candidate"],
                        help="Run only one case (for power-cycle-between-runs workflow)")
    parser.add_argument("--compare-only", action="store_true",
                        help="Skip running, just compare existing artifacts")
    parser.add_argument("--no-restore", action="store_true",
                        help="Do not restore live gklib after run (useful with --case)")
    parser.add_argument("--wait-boot", action="store_true",
                        help="Wait for printer to come online (after power cycle)")
    parser.add_argument("--boot-timeout", type=int, default=180,
                        help="Seconds to wait for printer boot (default: 180)")
    args = parser.parse_args()

    artifacts = pathlib.Path(args.artifacts_dir)
    artifacts.mkdir(parents=True, exist_ok=True)

    # Compare-only mode: just load existing artifact files and compare
    if args.compare_only:
        orig_shim_path = artifacts / "original.shim.log"
        cand_shim_path = artifacts / "candidate.shim.log"
        missing = []
        if not orig_shim_path.exists():
            missing.append(str(orig_shim_path))
        if not cand_shim_path.exists():
            missing.append(str(cand_shim_path))
        if missing:
            print(f"ERROR: missing artifact files: {', '.join(missing)}", file=sys.stderr)
            print("Run --case original and --case candidate first.", file=sys.stderr)
            return 1

        orig_frames = parse_shim_log(orig_shim_path.read_text())
        cand_frames = parse_shim_log(cand_shim_path.read_text())
        comparison = compare_frames(orig_frames, cand_frames)
        for line in comparison:
            print(line)
        summary_path = artifacts / "comparison.txt"
        summary_path.write_text("\n".join(comparison) + "\n")
        print(f"\ncomparison saved to {summary_path}")
        return 0

    if args.wait_boot:
        print("waiting for printer to come online after power cycle...")
        remote = wait_for_printer(args.host, args.user, args.password, args.boot_timeout)
    else:
        remote = RemoteRunner(args.host, args.user, args.password)

    try:
        # The boot process starts gklib which configures the MCU.
        # To get a clean MCU state, we need the MCU to be in identify mode
        # (not configured, not shutdown). The reliable approach is:
        #   1. Kill gklib (frees the serial port)
        #   2. Wait for MCU software watchdog to fire (~5s -> SHUTDOWN)
        #   3. Wait for MCU hardware watchdog (STM32 IWDG) to fire
        #      (~5-8s after SHUTDOWN -> full hardware reset -> identify mode)
        #   Total: ~15-20s after killing gklib.
        #
        # FIRMWARE_RESTART doesn't work because boot gklib reconnects and
        # reconfigures the MCU before we can kill it (race condition).
        if not args.skip_reset:
            print("killing boot gklib to free serial ports...")
            stop_all(remote)
            # Wait for MCU hardware watchdog to fire and reset MCU to identify mode.
            # STM32 IWDG + Klipper software watchdog: total ~15-20s.
            # Use 25s for safety margin.
            watchdog_wait = 25
            print(f"  waiting {watchdog_wait}s for MCU hardware watchdog reset...")
            time.sleep(watchdog_wait)
            print("  MCU should now be in clean identify mode")
        else:
            print("stopping all gklib/shim processes (--skip-reset)...")
            stop_all(remote)
            time.sleep(2.0)

        # Prepare shimmed config
        target_cfg = remote.must_run(f"readlink -f {REMOTE_CFG_LINK}").strip()
        config_text = remote.read_text(target_cfg)
        shimmed = rewrite_mcu_serial(config_text, args.mcu_section, args.listen_port)
        remote.write_text(REMOTE_TEMP_CFG, shimmed)
        print(f"wrote shimmed config: {REMOTE_TEMP_CFG}")
        print(f"  [mcu] serial rewritten to tcp@127.0.0.1:{args.listen_port}")

        # Determine which cases to run
        if args.case:
            binary = REMOTE_ORIGINAL if args.case == "original" else REMOTE_CANDIDATE
            cases = [(args.case, binary)]
        else:
            cases = [
                ("original", REMOTE_ORIGINAL),
                ("candidate", REMOTE_CANDIDATE),
            ]
            if args.candidate_first:
                cases.reverse()

        results: dict[str, tuple[str, str]] = {}

        # Stop live gklib
        print("\nstopping live gklib...")
        stop_all(remote)

        for idx, (name, binary) in enumerate(cases):
            print(f"\n{'='*60}")
            print(f"RUNNING: {name} ({binary})")
            print(f"{'='*60}")

            # Between cases (not the first), need to get MCU back to identify mode.
            # Start gklib_real directly (not shimmed) to connect to MCUs,
            # wait for Ready, then FIRMWARE_RESTART + kill.
            if idx > 0 and not args.skip_reset:
                print("  resetting MCU between cases via firmware_restart...")
                restore_cmd = (
                    f"sh -lc 'cd {REMOTE_DIR} && export LD_LIBRARY_PATH={REMOTE_DIR}:$LD_LIBRARY_PATH; "
                    f"nohup {REMOTE_ORIGINAL} -a /tmp/unix_uds1 rinkhals_gklib.cfg "
                    f">/tmp/gklib_between_reset.out 2>&1 & echo $!'"
                )
                between_pid = start_background(remote, restore_cmd)
                print(f"  started temp gklib_real PID={between_pid}, waiting for Ready...")
                ready = wait_for_gklib_ready(remote, timeout=60)
                if ready:
                    firmware_restart_via_api(remote)
                    stop_all(remote)
                    print("  waiting 5s for MCUs to enter identify mode...")
                    time.sleep(5.0)
                else:
                    stop_all(remote)
                    print("  waiting 30s for MCU watchdog fallback...")
                    time.sleep(30.0)

            shim_text, gklib_text = run_case(
                remote, name, binary,
                args.listen_port, args.baud,
                args.serial_device, args.duration,
            )

            # Save locally
            (artifacts / f"{name}.shim.log").write_text(shim_text)
            (artifacts / f"{name}.gklib.out").write_text(gklib_text)
            results[name] = (shim_text, gklib_text)

            # Brief status from gklib output
            for needle in ["project state: Ready", "project state: Shutdown",
                           "Timeout on connect", "TMC", "tmcuart", "IFCNT"]:
                count = gklib_text.count(needle)
                if count:
                    print(f"  gklib output contains '{needle}': {count} times")

            # Diagnostic: check TX frame timeline to verify config commands were sent
            frames = parse_shim_log(shim_text)
            tx_frames = [f for f in frames if f.direction == "tcp->serial"]
            rx_frames = [f for f in frames if f.direction == "serial->tcp"]
            print(f"  shim total: {len(tx_frames)} tx, {len(rx_frames)} rx frames")
            if tx_frames:
                print(f"  tx range: {tx_frames[0].timestamp} - {tx_frames[-1].timestamp}")
            if len(tx_frames) < 100:
                print(f"  WARNING: only {len(tx_frames)} tx frames - "
                      "MCU may not have been in clean identify mode")

            stop_all(remote)

        # If single case mode, try to load the other from existing artifacts
        if args.case:
            other = "candidate" if args.case == "original" else "original"
            other_path = artifacts / f"{other}.shim.log"
            if other_path.exists():
                print(f"\nloading existing {other} artifacts for comparison...")
                results[other] = (other_path.read_text(), "")
            else:
                print(f"\nno existing {other}.shim.log -- run --case {other} after power cycle to compare")

        # Compare if we have both
        if "original" in results and "candidate" in results:
            print(f"\n{'='*60}")
            print("COMPARISON")
            print(f"{'='*60}")

            orig_frames = parse_shim_log(results["original"][0])
            cand_frames = parse_shim_log(results["candidate"][0])

            comparison = compare_frames(orig_frames, cand_frames)
            for line in comparison:
                print(line)

            summary_path = artifacts / "comparison.txt"
            summary_path.write_text("\n".join(comparison) + "\n")
            print(f"\ncomparison saved to {summary_path}")

    finally:
        if not args.no_restore:
            # Restore live service
            print("\nrestoring live gklib...")
            stop_all(remote)
            time.sleep(1.0)
            try:
                restore_cmd = (
                    f"sh -lc 'cd {REMOTE_DIR} && export LD_LIBRARY_PATH={REMOTE_DIR}:$LD_LIBRARY_PATH; "
                    "nohup ./gklib -a /tmp/unix_uds1 rinkhals_gklib.cfg >/tmp/gklib.log 2>&1 & echo $!'"
                )
                pid = start_background(remote, restore_cmd)
                print(f"  restored live gklib: PID={pid}")
                time.sleep(5.0)
                ps, _, _ = remote.run("ps | grep './gklib -a /tmp/unix_uds1' | grep -v grep")
                print(f"  live process: {ps.strip()}")
            except Exception as e:
                print(f"  WARNING: failed to restore live gklib: {e}", file=sys.stderr)
        else:
            print("\nskipping live gklib restore (--no-restore)")
        remote.close()

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
