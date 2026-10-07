# ADR 0009: Agent Plugins packaging of the harness integration

Status: accepted. Refines ADR 0005 ("Host-delegated proofs", the harness sections and
the channel selection).

## Context

ADR 0005 ships one Claude Code plugin per module and writes Codex and Copilot files
into the project with `init`. Copilot CLI has no usable plugin that way: its agents,
skill and hooks are project files, and nothing validates them as a package. The open
standard Agent Plugins 1.0.0 (specification repository `agentplugins/agent-plugins-spec`,
tag `v1.0.0`, commit `bd383552095128f6effe895b9257cfd580a6d179`) defines a portable
package that Copilot CLI and VS Code, Codex, Cursor and Kiro load.

The standard has exactly two portable component types and one extension mechanism:

- `plugin.json` at the root, with a closed set of top-level fields: `$schema`, `name`,
  `version`, `description`, `author`, `homepage`, `repository`, `license`, `keywords`,
  `extensions`. `$schema` is `https://agent-plugins.org/schemas/1.0.0/plugin.schema.json`.
- `skills/<name>/SKILL.md` in the Agent Skills format
  (`agentskills/agentskills`, `docs/specification.mdx` at commit
  `69ef37e9424c0a7ea9dd2293b559e43ec8176379`).
- `mcp.json` at the root, with `$schema`
  `https://agent-plugins.org/schemas/1.0.0/mcp.schema.json` and `mcpServers` whose
  entries are `stdio`, `streamable-http` or `sse`. A stdio `command` is one token: a
  bare name or a `./` path. A stdio server starts in the plugin root unless `cwd`
  says otherwise.
- Client-owned data under `extensions.<reverse-domain>` and client-owned files under
  a top-level directory of that name. Agents and hooks are not portable.

Claude Code does not read Agent Plugins. Copilot CLI reads agents and hooks from
`com.github.copilot/`. Codex reads `extensions.com.openai`, whose hooks apply to
manually installed desktop plugins only, and documents no plugin-scoped agents.

## Decisions

### One tree per plugin, portable core first

Every plugin of the repository (the three proof modules, the relay, the umbrella)
ships `plugin/portable/` (`umbrella/plugin/portable/` for the umbrella):

| Path | Content |
|---|---|
| `plugin.json` | identity, valid against the pinned schema; no `extensions` data |
| `skills/<binary>-fulfil/SKILL.md` | the fulfil skill, Agent Skills format, no `allowed-tools` |
| `mcp.json` | the executor server of a proof module; the relay and the umbrella declare none |
| `com.github.copilot/agents/*.agent.md`, `com.github.copilot/hooks/hooks.json` | restricted executor agents and hooks, for Copilot CLI |

The portable core is sufficient alone: the fulfil skill drives `fetch_request` and
`submit_response` through the MCP server without a subagent.

### What is portable and what is per harness

| Concern | Portable core | Per harness |
|---|---|---|
| Identity, skill, MCP server | yes | |
| Restricted executor subagents (two tools only) | | Claude Code `agents/`; Codex `.codex/agents/*.toml`; Copilot `com.github.copilot/agents/` |
| Hooks (capability issue, stop attestation) | | Claude Code `hooks/hooks.json`; Codex `.codex/hooks.json`; Copilot `com.github.copilot/hooks/hooks.json` |
| Packaging | `plugin/portable` | `plugin/claude` with `.claude-plugin/`; Codex files written by `init` |

The generator `format/hostproto/pluginfiles` renders every tree from one `Module`
value; `script/plugin-gen` writes the committed trees and each module's test fails
when a committed tree differs from its generator. Codex hooks and agents stay in the
files `init` writes, because the manifest-level hooks of Codex plugins are limited to
manually installed desktop plugins and would fire a second time beside the project
hooks.

### Isolation of the portable server

The isolation of an answer comes only from the launch arguments of the server that
took it, never from skill text. The root `mcp.json` launches `<binary> mcp --role
executor` with no `--harness`; an absent `--harness` selects the `portable` harness:
channel `host-inline`, isolation `inline`, attestation waived, harness name
`agent-plugins`. An answer given through it is an answer of the main agent, with block
power only (ADR 0005, "Claim power"), journaled as exposed.

The restricted executor agents of Claude Code, Codex and Copilot launch their own
agent-scoped server with `--harness claude|codex|copilot`; that server keeps the
channel `host-subagent` and the isolation its harness enforces. In Copilot the root
server and the agent-scoped server carry different names, so the agents' tool
allowlists and the hook matchers match only the restricted instance.

The one fulfil skill of the portable tree delegates to the executor agent of the
module when the client offers one and otherwise answers inline.

### Project root of the portable server

A stdio server of an Agent Plugin starts in the plugin root, and the specification
gives no placeholder for the project. The server resolves the project root from, in
order, `--root`, `CLAUDE_PROJECT_DIR`, the first `file://` root of MCP `roots/list`
when the client declares the `roots` capability, then the working directory. When the
resolved directory holds no `.spec-proof/config.yaml` the tools answer with a refusal
that says so.

### Schema validation in CI

The two official schemas are vendored under `format/hostproto/pluginfiles/schemas/`
with their source URL and SHA-256 digest recorded next to them. A test validates every
committed root `plugin.json` and `mcp.json` of an Agent Plugins tree against them
without network access, checks the digests, and validates the frontmatter of every
committed portable skill against the Agent Skills constraints (name equal to its
directory, at most 64 characters, description at most 1024). The test asserts the
expected list of trees so that an empty walk cannot pass.

### Copilot live canary

`script/canary-copilot` runs one short `copilot -p` session against the portable
trees through `--plugin-dir`, with `COPILOT_HOME`, `COPILOT_CACHE_HOME` and `--log-dir`
in a temporary directory, in the nightly window, under the same session rules as the
canaries of Claude Code and Codex. Copilot's credentials are read from the system
keychain, or from `COPILOT_GITHUB_TOKEN`, `GH_TOKEN` or `GITHUB_TOKEN` when one is set,
which the canary passes through unchanged; a `--shared-home` flag lets a session use
`~/.copilot` and then lists what the session wrote there. The user's `~/.copilot` is
fingerprinted before and after in both modes.

The session runs on `claude-haiku-4.5` with a cap of 60 AI credits and 40 model steps,
and never inherits `COPILOT_ALLOW_ALL`. Before it, `copilot plugin list --json` and
`copilot skill list --json` run with the three trees mounted, and each tree is
inspected. The session delegates one ticket to the executor agent, which is also asked
for a shell command outside its two tools; the hooks, the executor tool names, the
`agentType` of `preToolUse` and the allowlist are concluded from the recorded hooks and
the event stream, and a check whose evidence is absent concludes INCONCLUSIVE.

## Consequences

- A client that reads Agent Plugins gets the identity, the skill and the server from
  one tree without a per-client adapter; the capabilities that need a client's hooks
  and agents stay in that client's namespace.
- Answers through the portable server are inline, so a project that requires a
  restricted isolation for pass credit gets none from a client without the
  restricted executor agents, by the same rule that governs every inline answer.
- The vendored schemas change only through a new pinned release of the standard and
  a new ADR.
