#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"$ROOT/run_seed_demo.sh" 12345 99999
"$ROOT/run_stress_demo.sh" 12345 99999 256 3000
"$ROOT/run_synctest_demo.sh" 12345 99999
"$ROOT/run_fuzz_race_demo.sh" 1 80 3000 8 24

echo "All deterministic demos passed."
