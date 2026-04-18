# Motion Telemetry Diff Tool

This repository includes a CLI tool for A/B motion parity comparisons:

- Command: `cmd/motion_telemetry_diff`
- Metrics covered:
  - Segment timing (`segment_time`)
  - Flush cadence (`flush_time` deltas)
  - Queue depth (`queue_depth`)
  - Flush deadline slack (`need_flush_time - time`)
  - Step generation deadline slack (`need_step_gen_time - time`)

## Input formats

Each input file may be either:

1. JSON array of samples, or
2. NDJSON (one JSON sample per line)

Sample schema:

```json
{
  "time": 12.34,
  "segment_time": 0.05,
  "flush_time": 12.6,
  "queue_depth": 3,
  "need_flush_time": 12.9,
  "need_step_gen_time": 13.0
}
```

## Usage

```text
go run ./cmd/motion_telemetry_diff -baseline baseline.ndjson -candidate candidate.ndjson
```

Flags:

- `-baseline`: baseline telemetry path (required)
- `-candidate`: candidate telemetry path (required)
- `-pretty`: pretty JSON output (default `true`)

The output contains `baseline`, `candidate`, and `delta` blocks with the same metric fields.
