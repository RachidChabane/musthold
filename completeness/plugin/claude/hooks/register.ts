import type { EngineInterface, Register } from "claude-code"

// The binary decides what to classify and what to do with the answers; this
// module only carries a plan to the session's model and the answers back.
const BINARY = "spec-proof-completeness"
const PROVE_COMMAND = BINARY + " prove"
const SYSTEM_ONE_COMMAND = "systemone"
const PLAN_SUBCOMMAND = "classify-plan"
const IMPORT_SUBCOMMAND = "import"
const SUCCESS_EXIT_CODE = 0
const MAX_ATTEMPTS = 2

type PlannedCall = {
  question: string
  i: number
  model: string
  text: string
  labels: string[]
}

type PlannedItem = {
  ticket: string
  nonce: string
  source: string
  calls: PlannedCall[]
}

type CallResult = {
  question: string
  i: number
  model: string
  label: string | null
  attempts: number
}

// At most one run is in flight; a trigger that arrives meanwhile does nothing.
let isRunning = false

function isProveCommand(command: unknown): boolean {
  if (typeof command !== "string") return false
  const trimmed = command.trim()
  return trimmed === PROVE_COMMAND || trimmed.startsWith(PROVE_COMMAND + " ")
}

function isPlannedCall(value: any): boolean {
  return (
    value !== null &&
    typeof value === "object" &&
    typeof value.question === "string" &&
    typeof value.i === "number" &&
    typeof value.model === "string" &&
    typeof value.text === "string" &&
    Array.isArray(value.labels) &&
    value.labels.every((label: unknown) => typeof label === "string")
  )
}

function isPlannedItem(value: any): boolean {
  return (
    value !== null &&
    typeof value === "object" &&
    typeof value.ticket === "string" &&
    typeof value.nonce === "string" &&
    typeof value.source === "string" &&
    Array.isArray(value.calls) &&
    value.calls.every(isPlannedCall)
  )
}

// An unparsable plan, or one that is not a list, is an empty plan.
function parsePlan(stdout: string): PlannedItem[] {
  try {
    const parsed: unknown = JSON.parse(stdout)
    return Array.isArray(parsed) ? (parsed.filter(isPlannedItem) as PlannedItem[]) : []
  } catch {
    return []
  }
}

function resultOf(call: PlannedCall, label: string | null, attempts: number): CallResult {
  return { question: call.question, i: call.i, model: call.model, label, attempts }
}

// A rejection and an undefined label each earn one more attempt with the same
// arguments; a call that fails every attempt answers null.
async function classifyWithRetry($: EngineInterface, call: PlannedCall): Promise<CallResult> {
  for (let attempt = 1; attempt <= MAX_ATTEMPTS; attempt += 1) {
    try {
      const label = await $.model.classify(call.text, call.labels, { model: call.model })
      if (label !== undefined) return resultOf(call, label, attempt)
    } catch {
      // The next attempt, or the null answer, follows.
    }
  }
  return resultOf(call, null, MAX_ATTEMPTS)
}

async function answerItem(
  $: EngineInterface,
  item: PlannedItem,
  model: string,
  cliVersion: string,
  cwd: string,
): Promise<void> {
  const calls = await Promise.all(item.calls.map((call) => classifyWithRetry($, call)))
  await $.process.run(
    [BINARY, SYSTEM_ONE_COMMAND, IMPORT_SUBCOMMAND, item.ticket, "--nonce", item.nonce, "--source", item.source, "--root", cwd],
    { cwd, stdin: JSON.stringify({ cliVersion, sessionModel: model, calls }) },
  )
}

// A failure anywhere leaves the tickets pending for the next trigger.
async function run($: EngineInterface): Promise<void> {
  if (isRunning) return
  isRunning = true
  try {
    const model = await $.session.model()
    const cwd = await $.session.cwd()
    const cliVersion = (await $.session.version()).version
    const planned = await $.process.run(
      [BINARY, SYSTEM_ONE_COMMAND, PLAN_SUBCOMMAND, "--model", model, "--root", cwd],
      { cwd },
    )
    if (planned.exitCode !== SUCCESS_EXIT_CODE) return
    for (const item of parsePlan(planned.stdout)) {
      await answerItem($, item, model, cliVersion, cwd)
    }
  } catch {
    // Nothing is thrown into the session.
  } finally {
    isRunning = false
  }
}

export const register: Register = (on) => {
  on("tool.call", { tool: "Bash" }, async ($, e, next) => {
    const result = await next(e)
    if (result.deny === undefined && isProveCommand(e.command)) await run($)
    return result
  }).catch(($, e, next) => next(e))

  on("turn.start", async ($, e, next) => {
    const result = await next(e)
    await run($)
    return result
  })
}
