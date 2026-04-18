# Live Motion A/B Validation Procedure

This procedure records and compares smoothness-sensitive telemetry between a baseline build and a candidate build.

## Preconditions

- Same printer hardware and mechanical state for both runs.
- Same printer configuration and same G-code test path.
- Baseline and candidate binaries both deployable.
- Telemetry capture available in either JSON array or NDJSON format with fields:
  - `time`
  - `segment_time`
  - `flush_time`
  - `queue_depth`
  - `need_flush_time`
  - `need_step_gen_time`

## Steps

1. Run baseline build and capture telemetry to `baseline.ndjson`.
2. Run candidate build and capture telemetry to `candidate.ndjson`.
3. Compare both captures:

```text
build/live_ab_validation.sh baseline.ndjson candidate.ndjson
```

4. Review `build/live_ab_validation_report.json`.

## Metrics to compare

From `delta` in the report:

- `segment_timing_mean`, `segment_timing_p95`
- `flush_cadence_mean`, `flush_cadence_p95`
- `queue_depth_mean`, `queue_depth_max`
- `flush_deadline_slack_mean`, `flush_deadline_slack_min`
- `step_deadline_slack_mean`, `step_deadline_slack_min`

## Practical interpretation

- Lower segment timing values can indicate smoother segment scheduling.
- Large positive queue-depth deltas can indicate buffering drift.
- Deadline slack minima moving toward zero or negative indicates risk of timing starvation.
- Changes should be judged together with real print observations (vibration, ringing, homing reliability).

## Notes

- Run multiple passes to avoid one-off noise.
- Keep filament, temperatures, and acceleration settings constant between A and B runs.
- If telemetry has missing fields, ensure both baseline and candidate are missing the same fields before interpreting deltas.
