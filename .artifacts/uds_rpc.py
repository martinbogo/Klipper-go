#!/usr/bin/env python3
import argparse
import json
import socket


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--socket', default='/tmp/unix_uds1')
    ap.add_argument('--method', required=True)
    ap.add_argument('--script', default='')
    ap.add_argument('--id', type=int, default=9001)
    args = ap.parse_args()

    params = {}
    if args.script:
        params['script'] = args.script

    payload = (json.dumps({'id': args.id, 'method': args.method, 'params': params}, separators=(',', ':')) + '\x03').encode()

    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.connect(args.socket)
    sock.sendall(payload)

    data = b''
    while True:
        chunk = sock.recv(65536)
        if not chunk:
            break
        data += chunk
        if b'\x03' in data:
            data = data.split(b'\x03', 1)[0]
            break
    sock.close()

    print(data.decode('utf-8', 'replace'))


if __name__ == '__main__':
    main()
