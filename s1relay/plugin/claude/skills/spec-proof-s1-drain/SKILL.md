---
name: spec-proof-s1-drain
description: Relay the closed questions of the spec-proof modules to the System One endpoint of the project. Use when a module has closed requests waiting and `.spec-proof/config.yaml` sets `systemone.endpoint`.
allowed-tools: Bash(spec-proof-s1 drain *)
---

# Relay the closed requests

`spec-proof-s1 drain` posts the closed questions a module exported to the System One endpoint of the project and hands the answers back to the module. It is the only code of spec-proof that opens a connection to a model service.

1. If `.spec-proof/config.yaml` sets no `systemone.endpoint`, stop: the modules answer their closed requests with host votes.
2. For each module that is installed, run `spec-proof-s1 drain --module <module>` with the name of the module: `dp`, `alignment` or `completeness`. A failure leaves the requests of that module for its own fulfil skill.
3. Report how many requests were answered and how many remain.
