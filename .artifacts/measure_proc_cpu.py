#!/usr/bin/env python3
import argparse
import time


def read_cpu_total():
    with open('/proc/stat', 'r', encoding='utf-8', errors='replace') as f:
        fields = f.readline().split()[1:]
    return sum(int(x) for x in fields)


def read_proc_total(pid: int):
    with open(f'/proc/{pid}/stat', 'r', encoding='utf-8', errors='replace') as f:
        fields = f.read().split()
    return int(fields[13]) + int(fields[14])


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--pid', type=int, required=True)
    ap.add_argument('--samples', type=int, default=20)
    ap.add_argument('--interval', type=float, default=1.0)
    args = ap.parse_args()

    values = []
    for _ in range(args.samples):
        t0 = read_cpu_total()
        p0 = read_proc_total(args.pid)
        time.sleep(args.interval)
        t1 = read_cpu_total()
        p1 = read_proc_total(args.pid)
        dt = t1 - t0
        dp = p1 - p0
        cpu = (dp * 100.0 / dt) if dt > 0 else 0.0
        values.append(cpu)
        print(f'{cpu:.2f}')

    avg = sum(values) / len(values) if values else 0.0
    mn = min(values) if values else 0.0
    mx = max(values) if values else 0.0
    print(f'avg={avg:.2f} min={mn:.2f} max={mx:.2f} n={len(values)}')


if __name__ == '__main__':
    main()
