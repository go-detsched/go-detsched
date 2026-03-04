# Raft Instructional Patch Series

This directory stores a teaching-oriented sequence of patches that progressively
fix vulnerabilities in the intentionally vulnerable Raft demo.

Each stage has a deterministic proof contract:

1. run scenario in vulnerable mode (`--expect-bug=true`) and confirm the bug,
2. apply the stage patch transformation,
3. run scenario in fixed mode (`--expect-bug=false`) and confirm the fix.

The stage order and expected issue codes are defined in `stages.tsv`.

Patch files are preserved as instructional artifacts. The CI/local verifier
(`scripts/run-raft-patch-series-ci.sh`) applies equivalent stage codemods in
numeric order so bug-then-fix proofs stay deterministic across refactors.
