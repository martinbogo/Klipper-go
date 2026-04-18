#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  echo "Usage: $0 <baseline-telemetry.json|ndjson> <candidate-telemetry.json|ndjson> [output-report.json]" >&2
  exit 2
fi

baseline="$1"
candidate="$2"
output="${3:-build/live_ab_validation_report.json}"

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

mkdir -p "$(dirname "$output")"

echo "[1/3] Comparing baseline and candidate telemetry..."
go run ./cmd/motion_telemetry_diff -baseline "$baseline" -candidate "$candidate" -pretty=true > "$output"

echo "[2/3] Report written to $output"

echo "[3/3] Validation run completed. Review delta metrics in the report and correlate with smoothness observations."
