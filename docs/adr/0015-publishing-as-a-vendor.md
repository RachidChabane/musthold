# ADR 0015: Publishing as a vendor

Status: accepted. Supersedes the publication order of ADR 0004 ("Publication order")
and ADR 0007: the repository is made public before any benchmark campaign. Every
other part of ADR 0004 and ADR 0007 stands as the design of a campaign that this
project does not run.

## Context

The product claims that a change accepted by the four proofs contains fewer defects
than a change accepted by tests alone, at a stated cost. ADR 0004 and ADR 0007 made
a measured, signed campaign the condition of publication.

Independent tool vendors do not work that way. A vendor publishes small,
reproducible studies that back its own claims. Large benchmarks come from third
parties, who confirm or refute them. A campaign also costs the model quota and the
machine time of a single maintainer, and it delays the moment anyone can use the
product, run it on their own code and check it.

Several choices end up inside module paths, signed statements and published history.
They cannot change after publication without breaking the format or its users, so
this record fixes them before the repository is made public.

## Decisions

### Evidence

- **The project publishes vendor evidence and a replication kit.**
  - Vendor evidence means small studies that are pre-registered, reproducible and
    compared with a baseline. Every attempt is reported, failures and zero results
    included.
  - The replication kit is the benchmark core (`bench/`), the frozen protocols and
    the commands that run them, so a third party can repeat a study on its own
    corpus.
- **The campaign of ADR 0004 and ADR 0007 is not run.** The pre-registrations
  `prereg-validity-w` and `prereg-calibration-a1` stay frozen and unrun; a later run
  starts from their tags.
- **The launch makes no claim without a recorded run behind it.**
  - Claimed at launch: the mechanism, each demonstration with the command that
    reproduces it, and the pilot's safety result with its scope (no false pass, on
    one fixture).
  - Stated as a hypothesis, with the frozen protocol that tests it: the reduction in
    defects and the agent's correction after a block.
- **Launch demonstrations.**
  - A recorded run of the agent loop on a fixture: a blocked criterion, the
    counterexample the gate returns, the agent's correction, and the gate passing.
  - Scans of this repository and of the author's own repositories.
  - Findings on a named third-party repository are published only after the notice
    to its maintainers set by the scan's demo protocol.

### Identity

- **Name.** The project keeps the name `spec-proof`. Its module paths stay
  `github.com/RachidChabane/spec-proof/<module>`, and its binaries keep the
  `spec-proof-` prefix. No GitHub repository, PyPI package, npm package or
  Homebrew formula holds the name. The name states what the product delivers: a
  spec with its proof.
- **Predicate and schema URIs.** The URIs under `https://rachid-chabane.com/`
  (`attestations/…` and `host-protocol/…`) stay as they are, and the author's site
  serves them.
  - Each path serves its JSON schema and a short readable page.
  - These files are generated deterministically from the schemas of `format` by a
    script of this repository, and are copied into the site's static files.
  - Keeping the URIs changes neither the format, nor its reference vectors, nor the
    frozen pilot harness, which cites them.
- **Licence.** Apache-2.0. The copyright holder is Rachid Chabane.
- **Commit identity.** Public commits carry the author's GitHub `noreply` address.

### Public history and data

- **A new public repository is built from a filtered history.** The private
  repository is renamed `spec-proof-archive` and stays private as the evidence
  archive: its commits, tags and RFC 3161 timestamps keep resolving there.
  - Changing the visibility of the private repository in place would expose its
    remote branches, its pull request refs, the logs of its self-hosted runner, and
    objects that a rewrite only made unreachable. A new repository holds only what
    it is given.
  - The filter is a script of this repository, so it can run again on the final
    commit. It rewrites the commit identity in the same pass.
  - A published map links each public commit to its archive commit, so a timestamp
    that cites an archive commit can be tied to the public one.
- **What the filter removes:**
  - `notes/`, the maintainer's working notes;
  - the raw model outputs of the pilot runs (`experiments/*/runs/*/raw`): they
    carry session ids and per-call costs;
  - `bench/mining/*/archive/`, the archived GitHub responses of the abandoned
    campaign: they carry third-party logins. The candidate, attrition and sample
    files drawn from them name only public repositories and pull requests, and
    stay;
  - the names of internal working sessions and the author's local paths, wherever
    they appear in messages or files.
- **What stays published:** the pre-registrations, fixtures, oracles, harness code,
  results and analyses, and the `SHA256SUMS` of every run.
  - A sum that covers a removed raw file can be checked only against the archive.
    The published results state this.

### Infrastructure of the public repository

- **Continuous integration runs on GitHub-hosted runners.** The self-hosted runner
  of the maintainer's machine is removed from every repository first, because a pull
  request from a fork could run arbitrary code on it.
- **Shallnot 1.2 is released before the launch.** Its proof gate is merged into the
  public Shallnot repository and released, so a public user can run the whole loop.
  CI then fetches Shallnot from its public release, with no deploy key.
- **Tags come last.** Module tags are pushed only once the public repository exists
  and its CI is green: the Go module proxy keeps a tag forever.

## Consequences

- The launch is not delayed by a campaign. What the product claims at launch is
  limited to what a recorded run shows.
- The replication kit and the frozen protocols are the invitation to third parties.
  A refutation by a third party is a result the project reports.
- The archive repository stays the only place where the original commits, the raw
  outputs and the archived GitHub responses can be checked. Losing it loses that
  evidence, not the published results.
- The URIs depend on the author's site staying online. Losing the site breaks no
  execution, since every schema is embedded in the binaries, but the URIs stop
  resolving.
- One risk stays open: the project reuses nothing from the client benchmark the
  author built at work, and an audit found no trace of it, but only the author can
  rule out the reuse of an idea.
