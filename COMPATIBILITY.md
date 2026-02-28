# Compatibility Matrix

| Detsched Repo Tag | Upstream Go Tag | Status |
|---|---|---|
| (unreleased) | `go1.26.0` | Verified: patch apply + build + `run_all_demos.sh` |

## Verification contract

For a compatibility row to be marked verified:

1. `git apply --check` succeeds.
2. Patch applies cleanly.
3. `src/make.bash` succeeds.
4. `misc/detscheddemo/run_all_demos.sh` succeeds.
