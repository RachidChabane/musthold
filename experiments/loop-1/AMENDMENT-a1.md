# Amendment a1 to the loop-1 pre-registration

This document amends `PREREG.md` as frozen under the tag `prereg-loop-1`. It is
frozen under the tag `prereg-loop-1-a1` before any construction call, canary
session or run. Where it differs from `PREREG.md`, it prevails; every other rule of
`PREREG.md` holds.

## A1. No time window (§6.3, §6.6, §6.7, §13.3)

- §6.3 is removed. A run may start at any time.
- §6.6: the calendar cap is 28 days after the start of the first run, in place of
  28 nights.
- §6.7: the limit of five runs started per night is removed. At most 72
  session-hours remain: 36 runs at the 7200 s cap.
- §13.3: the canary sessions are not bound to a window.
- §6.4: "the run started outside the window" is no longer a harness error.

## A2. Quota stops and attempts (§6.4, §6.5)

- A quota stop voids its attempt, and the invocation stops as §6.5 says. It does
  not count toward the limit of three attempts of §6.4.
- A harness error counts toward that limit. A run whose third attempt is a harness
  error is `harness-blocked`.
- A run is `quota-blocked` only when the calendar cap ends the run phase while the
  run's last attempt was a quota stop.

## A3. Quota patterns (§6.5, §13.3, §13.4)

The patterns that recognise a usage or rate limit in the CLI's wording are a fixed
list of the runner, at the commit tagged `loop-1-inputs`. The canary records that
list, and `inputs.json` holds it as `quota_patterns`.

## A4. Shallnot pin (§2)

The Shallnot commit is `c331f9b97d5b96bdc18eba3b17be870c004dc239`, the release
v0.5.1, as `script/shallnot.pin` holds at the tag `prereg-loop-1`.

## A5. Engine (§7.1)

The engine is the colima profile `musthold-loop-1`, with 2 CPUs, 4 GiB of memory
and a 30 GiB disk. Its only mounts are `$LOOP1_WORK`, read-write, and the product
directory `$LOOP1_PRODUCT`, read-only.

## A6. Credential (§7.5)

The runner writes the token to the standard input of the session container's entry
process, which sets `CLAUDE_CODE_OAUTH_TOKEN` in the environment of the session
process alone. The token is never in the docker client's environment, an argument,
a file, a log or a record.

## A7. Docker configuration (§7.1)

Starting the engine adds colima's own context entry for the profile under the
user's Docker configuration. The tooling never activates it, and changes no other
Docker configuration of the host.

## A8. Product sources (§2)

`build` compiles the product from the tree of the commit it runs at (`git archive`),
never from a working tree, and records whether the working tree differed.

## A9. Runs at the calendar cap (§6.4, §6.6)

When the calendar cap ends the run phase, a run with no counted attempt is
`not-run` if it never started, `quota-blocked` if its last attempt was a quota
stop, and `harness-blocked` if its last attempt was a harness error, whatever the
number of its attempts.

## A10. Oracle run killed for memory (§4.1, §4.3)

An oracle or held-out run whose container is killed for memory counts as a run that
reached its limit: every test fails in that run.

## A11. Edits of the run's records (§7.3, §9.8)

The session's container mounts the run directory read-write, records included. K4
also hits on an `Edit`, `Write`, `MultiEdit` or `NotebookEdit` call on a path under
the run directory's `records/` or `objects/`.

## A12. Container options and environment (§7.3, §7.4)

Every container of the study also runs with `--init`, `--read-only`, a tmpfs at
`/tmp` and at the image user's home, and `--log-driver none`. The tmpfs mounts are
not host paths.

Beside the variables of §7.4, every command receives git's trust of the project
directory (`GIT_CONFIG_COUNT`, `GIT_CONFIG_KEY_0=safe.directory`,
`GIT_CONFIG_VALUE_0=<run>/project`), and the session also receives the proxy
variables in lower case (`https_proxy`, `http_proxy`, `no_proxy`). The preparation's
git commands also receive the fixed identity and date of §3.5 and
`GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_SYSTEM=/dev/null`.

## A13. Plugin trees (§5, §13.3)

Both arms load six plugin trees with `--plugin-dir`: the three module plugins,
Shallnot's plugin, the relay's plugin and the umbrella plugin, which depends on the
other five. C3 holds its first condition when the session's `system/init` event
lists all six plugins and names the session model, and when every hook that a
module plugin's `hooks/hooks.json` declares as `<module binary> hook <kind>` was
called at least once, as `hooks.jsonl` records.

## A14. Acceptance check A8 (§3.4)

A8 compares lines of at least 20 characters after trimming that are not import
statements. These lines do not count: a line that a violation or a refactor shares
with the reference; an oracle line that restates one of the spec's inline examples;
an oracle line identical to a line of the fixture's own tests. The words of A8 match
with their endings.

## A15. Repairs during construction (§3.3)

A check among A3 to A6 that fails returns its output to both roles, and uses one
repair call of each. A model reply that fails the output schema uses a repair call
of its role. A call that fails for the harness (authentication, usage or rate
limit, network, time limit) stops `construct`, uses no repair and drops nothing;
the next invocation resumes at that call.

## A16. Implementation notes

`RUN_RECORDS.md`, frozen with this amendment, states the rule the tooling follows
wherever `PREREG.md` and this amendment leave a detail open.
