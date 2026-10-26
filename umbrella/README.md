# umbrella

The Claude Code marketplace `musthold`, the thin plugin `musthold` that
installs Shallnot and every musthold module and runs them together with one skill,
`prove-all`, and the `musthold` dispatcher, which runs each command in the binary
of the module that owns it: `musthold scan` runs `musthold-dp scan`. The plugin
holds no code, binary, hook, agent, command or server.

## Alone

The plugin works with whichever module binaries are on `PATH`: its first step runs
`doctor` of each module and names the ones that are missing, which never count as
proved.

## Install

```
/plugin marketplace add RachidChabane/musthold
/plugin install musthold@musthold
```

Plugins install no binary; build the module binaries as described in
[getting started](../docs/usage/getting-started.md#1-install-the-binaries).

## Files

The files under `umbrella/` and the marketplace at the repository root
(`.claude-plugin/marketplace.json`) are generated from one definition; regenerate
them with `script/plugin-gen`.

## Documentation

- [Umbrella](../docs/usage/umbrella.md)
- [Project README](../README.md)
