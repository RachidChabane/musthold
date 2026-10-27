# loop-1: the agent loop under the proof gate

A small pre-registered study. Claude Code, run headless, implements a task from a
spec in a fixture project, under Shallnot's end-of-turn gate. The two arms differ
only in what the gate requires:

- `control`: traceability alone;
- `treatment`: traceability, discriminating power and alignment.

A blind oracle, which the agent never sees, then measures three things:

- whether the treatment ships fewer defective changes;
- whether the agent corrects its tests and code after a block;
- what that costs.

[`PREREG.md`](PREREG.md) is the protocol. Every rule of the study is there.

## Layout

| Path | Content |
|---|---|
| `PREREG.md` | the protocol |
| `loop-1.json` | the run configuration: tasks, arms, repetitions, seed, model, caps, permission rules |
| `tasks/<task>/` | the truth files of each task: reference, violations, refactors, oracle |
| `image/` | the container image the runs use |
| `freeze/` | the frozen commits, their RFC 3161 timestamps, `inputs.json` and the sums |
| `results/` | the committed results |
| `script/`, `Makefile` | the front door |

## Front door

Each verb runs as `script/<verb>`, or as `make <verb>`, from this directory. Run
`make help` for the list. Every verb except `power` and `build` runs the runner
`musthold-demo-loop`, built from `tools/canary`, with the subcommand of the same
name. `build` builds the product, the runner and the image; `power` prints the
power table.

These verbs spend model quota:

- `construct`;
- `canary`;
- `run`.

`script/run --dry-run` prints every command of every run, in order, and starts
nothing.

## Replicate

The commands, the machine and the inputs a third party needs are in §14 of the
protocol. To run the study on other tasks, see §14, step 4.
