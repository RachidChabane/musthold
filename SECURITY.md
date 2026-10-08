# Security policy

## Report a vulnerability

Report a vulnerability privately through GitHub's private vulnerability reporting:
open the **Security** tab of the repository, choose **Report a vulnerability** and
fill in the form. Only the maintainers and you can read the report.

Do not open a public issue, pull request or discussion for a vulnerability.

A useful report gives:
- the module and the version (`<binary> --version`);
- what an attacker needs (a local agent, a pull request author, a network position);
- steps or a proof of concept that reproduce it;
- the effect: what the attacker obtains or what a gate wrongly accepts.

## Supported versions

Each module is released on its own, from tags named `<module>/vMAJOR.MINOR.PATCH`. The
latest release of each module receives security fixes. Earlier releases do not.

## Scope

In scope, because the product exists to make these hold:

- **Signatures.** Statements signed by CI and checked by `spec-proof-verify` against
  a trust policy: a forged, replayed or substituted statement admitted as signed, a
  trust policy bypass, a signature check that accepts a statement it should refuse.
- **Attestations and the ledger.** The append-only ledger kept by `spec-proof-ledger`
  and the provenance of release archives (checksums, Sigstore signatures, GitHub
  attestations, SBOMs): a tampered entry or archive that verifies, and an installer
  that installs an artifact it did not verify.
- **The host protocol.** The request, ticket and response types of `format/hostproto`,
  the executor server, the capability check and the hook payload decoders: an answer
  accepted without the capability or nonce it requires, a ticket resubmitted with a
  different output, a leak of the request to the executor's caller.
- **The gate integration.** The proofs and the verified set Shallnot reads: a missing,
  stale or local proof that passes a gate configured with `proofs.trust: signed`, and
  a judged signal that blocks or earns pass credit without the executed corroboration
  the trust model requires.

Out of scope:

- anything that needs write access to the developer's own machine, files or agent
  session: under `proofs.trust: local` the agent and the developer share filesystem
  access, and local proofs are advisory by design ([trust model](docs/usage/trust-model.md#limits));
- vulnerabilities in the harnesses (Claude Code, Codex CLI, Copilot CLI), in Shallnot
  or in a third-party dependency: report them to their own projects. A dependency
  issue that spec-proof makes exploitable is in scope;
- findings that require a weakened configuration the documentation warns against,
  such as `host.require_attestation: false` or a `proofs.min_isolation` below
  `restricted`.

## Response

The maintainers acknowledge a report, assess it and agree a disclosure date with the
reporter. A fix is released as a new version of each affected module, and a published
advisory names the modules and versions affected.
