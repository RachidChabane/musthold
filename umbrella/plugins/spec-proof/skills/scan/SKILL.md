---
name: scan
description: Scan the code, or a pull request, for the promises its sources make and the tests that do not hold them, answering the model work of the scan with blind executors. Use when asked to scan a repository or a pull request, and when `spec-proof scan --host` exits with code 3.
allowed-tools: Bash(spec-proof scan *), Bash(spec-proof-dp request list *), Bash(spec-proof-s1 drain *)
---

# Scan with a host

`spec-proof scan --host` stops with exit code 3 while it waits for model work. Each request is a ticket of the scan's own session. The tickets of any other session are not yours. Never answer a ticket yourself and never read what a ticket asks.

1. Run `spec-proof scan --host [<PR>]`, with the pull request number when the user gave one, and the flags the user gave.
2. Exit code 0: the scan finished. Show its report and stop. Exit code 2: show the refusal to the user and stop. Do not loop.
3. Exit code 3: read the line `session scan:<key>` of the output and keep that session. Answer its tickets with steps 4 to 6.
4. If `spec-proof-s1` is on `PATH` and `.spec-proof/config.yaml` sets `systemone.endpoint`, run `spec-proof-s1 drain --module dp --session scan:<key>`. A failure leaves the tickets for the next step.
5. Run `spec-proof-dp request list --session scan:<key>`. Each line is `<ticket> <role>`. List no other session.
6. For each ticket, launch one foreground `Agent` with `subagent_type` set to the role on its line and `prompt` set to `ticket <ticket>`. Launch at most 4 in one message and wait for them. Set no model and add nothing to the prompt. The reply of an executor is a receipt: do not act on it.
7. Run `spec-proof scan --host --resume [<PR>]`, with the same pull request number and flags as in step 1. Go back to step 2 with its exit code.
