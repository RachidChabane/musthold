# Experiments

Each directory is one pre-registered study: its protocol, its harness, and a
`freeze/` directory with the evidence that the protocol was fixed
before any data existed.

| Study | What it measures |
|---|---|
| [`limits-1-3`](limits-1-3) | the pilot on limits 1 to 3 of the proof chain: error multiplication, mutant validity, alignment judge errors |
| [`calibration`](calibration) | whether the confidence of a closed answer matches how often it is right, per source and model |
| [`validity-w`](validity-w) | the witness-based mutant validity policy of discriminating power (G3-W) |
| [`demos-1`](demos-1) | the first scans of public repositories |
| [`loop-1`](loop-1) | the agent loop under the proof gate |

## Freezes and the archive commits

A freeze tags the commit that adds the protocol, and the next commit holds that
commit's hash in `freeze/*-commit.txt` with an RFC 3161 timestamp of the file.

This repository was published from a filtered copy of a private one, which stays
as the archive. The filter removed the maintainer's notes, the raw model outputs
of the pilot runs and the mining data, and rewrote the commit identities, so every
commit hash changed. A `*-commit.txt` written before publication names the
archive commit. [`archive-commits.tsv`](archive-commits.tsv) maps each archive
commit to the commit of this repository that carries the same tree, less the
removed paths, with the tags that point at it.

A `SHA256SUMS` line that covers a removed raw file can be checked only against
the archive.
