from __future__ import annotations

import json
import os
import shlex
import time

import paramiko

REMOTE_SOCKET = '/tmp/unix_uds1'
LOG_CMD = "grep -E 'FIRMWARE_RESTART|G28 Y|DUMP_TMC|Unable to read tmc uart|Unable to write tmc uart|Timeout on wait for .*tmcuart_response|project state: Shutdown|project state: Ready|Homing failed|Internal error on command|IFCNT unchanged|transport timeout|reading IFCNT' /tmp/rinkhals/gklib.log 2>/dev/null | tail -n 220 || tail -n 220 /tmp/rinkhals/gklib.log 2>/dev/null || true"


class RemoteRunner:
    def __init__(self) -> None:
        self.client: paramiko.SSHClient | None = None
        self.connect()

    def connect(self) -> None:
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        client.connect(
            os.environ['PRINTER_HOST'],
            username=os.environ['PRINTER_USER'],
            password=os.environ['PRINTER_PASSWORD'],
            timeout=15,
            banner_timeout=15,
            auth_timeout=15,
        )
        self.client = client

    def reconnect_until_ready(self, timeout: int = 180) -> None:
        if self.client is not None:
            try:
                self.client.close()
            except Exception:
                pass
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                self.connect()
                return
            except Exception:
                time.sleep(3)
        raise RuntimeError('unable to reconnect to printer after firmware restart')

    def run(self, cmd: str, timeout: int = 60):
        assert self.client is not None
        stdin, stdout, stderr = self.client.exec_command(cmd, timeout=timeout)
        out = stdout.read().decode('utf-8', 'replace')
        err = stderr.read().decode('utf-8', 'replace')
        code = stdout.channel.recv_exit_status()
        return code, out, err

    def must_run(self, cmd: str, timeout: int = 60) -> str:
        code, out, err = self.run(cmd, timeout)
        if code != 0:
            raise RuntimeError(f'command failed ({code}): {cmd}\nSTDOUT:\n{out}\nSTDERR:\n{err}')
        return out

    def close(self) -> None:
        if self.client is not None:
            self.client.close()
            self.client = None


class WebhookClient:
    def __init__(self, remote: RemoteRunner) -> None:
        self.remote = remote
        self.python_bin = self.remote.must_run("sh -lc 'command -v python3 || command -v python'", timeout=15).strip().splitlines()[-1]
        self.request_id = 3000

    def request(self, method: str, params: dict, timeout: int = 60):
        self.request_id += 1
        payload = json.dumps({'id': self.request_id, 'method': method, 'params': params}, separators=(',', ':'))
        remote_script = f"""
import json
import socket
payload = {payload!r}.encode() + b'\\x03'
sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
sock.settimeout({timeout})
sock.connect({REMOTE_SOCKET!r})
sock.sendall(payload)
data = b''
while True:
    chunk = sock.recv(65536)
    if not chunk:
        break
    data += chunk
    if b'\\x03' in data:
        data = data.split(b'\\x03', 1)[0]
        break
sock.close()
print(data.decode('utf-8', 'replace'))
"""
        cmd = f"{shlex.quote(self.python_bin)} - <<'PY2'\n{remote_script}\nPY2"
        code, out, err = self.remote.run(cmd, timeout=timeout + 20)
        if code != 0:
            raise RuntimeError(f'request {method} failed ({code}) stdout={out!r} stderr={err!r}')
        text = out.strip()
        if not text:
            raise RuntimeError(f'empty response for {method}')
        return json.loads(text)

    def wait_ready(self, timeout: int = 180):
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                resp = self.request('Query/K3cInfo', {}, timeout=20)
                if resp.get('result', {}).get('ready') is True:
                    return resp
            except Exception:
                pass
            time.sleep(2)
        raise RuntimeError('printer did not become ready after firmware restart')


def main() -> int:
    remote = RemoteRunner()
    try:
        client = WebhookClient(remote)
        result: dict[str, object] = {'steps': []}

        restart_entry: dict[str, object] = {'name': 'firmware_restart'}
        try:
            restart_entry['response'] = client.request('gcode/script', {'script': 'FIRMWARE_RESTART'}, timeout=30)
        except Exception as exc:
            restart_entry['exception'] = str(exc)
        remote.reconnect_until_ready(180)
        client = WebhookClient(remote)
        restart_entry['ready'] = client.wait_ready(180)
        code, out, err = remote.run(LOG_CMD, timeout=30)
        restart_entry['log_tail'] = out
        restart_entry['log_tail_stderr'] = err
        result['steps'].append(restart_entry)

        commands = [
            ('dump_y_ifcnt', 'DUMP_TMC STEPPER=stepper_y REGISTER=IFCNT\nM400', 60),
            ('dump_y_gconf', 'DUMP_TMC STEPPER=stepper_y REGISTER=GCONF\nM400', 60),
            ('dump_x_ifcnt', 'DUMP_TMC STEPPER=stepper_x REGISTER=IFCNT\nM400', 60),
            ('g28_y', 'G28 Y\nM400', 240),
        ]
        for name, script, timeout in commands:
            entry: dict[str, object] = {'name': name, 'script': script}
            try:
                entry['response'] = client.request('gcode/script', {'script': script}, timeout=timeout)
            except Exception as exc:
                entry['exception'] = str(exc)
            try:
                entry['ready'] = client.request('Query/K3cInfo', {}, timeout=20)
            except Exception as exc:
                entry['ready_error'] = str(exc)
            try:
                entry['toolhead'] = client.request('objects/query', {'objects': {'toolhead': None}}, timeout=20)
            except Exception as exc:
                entry['toolhead_error'] = str(exc)
            code, out, err = remote.run(LOG_CMD, timeout=30)
            entry['log_tail'] = out
            entry['log_tail_stderr'] = err
            result['steps'].append(entry)
            if entry.get('ready', {}).get('result', {}).get('ready') is False:
                break

        print(json.dumps(result, indent=2, sort_keys=True))
        return 0
    finally:
        remote.close()


if __name__ == '__main__':
    raise SystemExit(main())
