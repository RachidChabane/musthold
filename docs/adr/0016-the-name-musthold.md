# ADR 0016: The name Musthold

Status: accepted. Supersedes the "Name" decision of ADR 0015. Every other decision
of ADR 0015 stands.

## Context

ADR 0015 kept the working name `spec-proof`. The name is free on every registry.
It is also generic: it describes a category rather than a product, and a
neighbouring project already publishes as "spec-to-proof". A launch whose aim is to
be noticed needs a name that is distinctive and that people can say aloud.

The product works beside Shallnot, whose name is the RFC 2119 keyword SHALL NOT.
Shallnot proves that each requirement is traced to its tests. This product proves
that those tests discriminate: the requirement must hold, and a change that breaks
it cannot pass.

Before publication, the name is cheap to change. After the first module tag, the
Go module proxy keeps the module path forever.

## Decision

- **The product is named Musthold.** It takes the RFC 2119 keyword MUST and the
  property every requirement is proven to have: it holds. The pair reads as one
  family: Shallnot traces, Musthold proves.
- **Checked free:** no GitHub repository named `musthold`, no PyPI, npm or Homebrew
  package, and no software product under that name. `musthold.dev` and
  `musthold.io` have no DNS delegation. `musthold.com` is parked for sale.
- **What changes before publication:**
  - the repository becomes `github.com/RachidChabane/musthold`;
  - the module paths become `github.com/RachidChabane/musthold/<module>`;
  - the binaries change their prefix from `spec-proof-` to `musthold-`;
  - the plugin and marketplace names, the configuration key and the cache
    directory change to match;
  - the docs and the README change to the new name.
- **What does not change:**
  - The predicate and schema URIs under `https://rachid-chabane.com/`. They carry
    no product name, so the format, its reference vectors and the frozen pilot
    harness are untouched.
  - Frozen pre-registrations, their tags and the recorded runs. They name
    `spec-proof` as the product they ran, which stays true of their history.
- **Order.** The rename lands on `main` after the launch branches are integrated,
  in one change with no alias for the old name. It lands before the public
  repository is built, so the public history never publishes a tag under the old
  module path.

## Consequences

- The archive repository keeps the old name in its history. ADR 0015's history
  filter needs no new rule for the name.
- Users of the unpublished `spec-proof` paths: there are none outside this
  repository, so no compatibility shim is kept.
- The name is not registered as a trademark. If a conflict appears before launch,
  it is handled here, by a new ADR.
