import { expect, test } from "claude-code/testing"

const BINARY = "musthold-sample"
const SESSION_MODEL = "session-model"
const SESSION_CWD = "/work/project"
const CLI_VERSION = "2.1.295"
const PROVE_COMMAND = BINARY + " prove --all"
const NONCE = "nonce-4f9a1c"
const TICKET = "ticket-7"
const SOURCE = "closed-source"
const FAILURE_EXIT_CODE = 2
const MAX_ATTEMPTS = 2
const TEST_TIMEOUT_MS = 2000

const FORBIDDEN_EVENTS = [
  "store.set",
  "store.delete",
  "fs.write",
  "ui.toast",
  "ui.log",
  "ui.status",
  "ui.notice",
  "prompt.submit",
  "http.fetch",
  "session.append",
  "model.complete",
]

type Call = { question: string; i: number; model: string; text: string; labels: string[] }
type Item = { ticket: string; nonce: string; source: string; scheme: string; k: number; mode: string; calls: Call[] }

function call(question: string, i: number, text: string): Call {
  return { question, i, model: "planned-model-" + i, text, labels: ["yes", "no"] }
}

function item(calls: Call[], ticket = TICKET): Item {
  return { ticket, nonce: NONCE, source: SOURCE, scheme: "scheme", k: calls.length, mode: "auto", calls }
}

type World = {
  classifications: { text: string; labels: readonly string[]; model: string | undefined }[]
  runs: { argv: readonly string[]; stdin: string | undefined; cwd: string | undefined }[]
  forbidden: string[]
  started: () => number
}

type Behaviour = {
  plan: Item[] | string
  planExitCode?: number
  classify?: (text: string, attempt: number) => string | undefined | Error
  holdUntilAllStarted?: number
}

function stage(on: any, behaviour: Behaviour): World {
  const world: World = { classifications: [], runs: [], forbidden: [], started: () => world.classifications.length }
  const attempts = new Map<string, number>()
  let release: () => void = () => {}
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })

  on("session.model", async () => ({ value: SESSION_MODEL }))
  on("session.cwd", async () => ({ value: SESSION_CWD }))
  on("session.version", async () => ({ value: { version: CLI_VERSION } }))
  on("tool.call", async () => ({ result: "", text: "", isError: false }))
  on("turn.start", async (_$: any, e: any) => ({ turnId: e.turnId }))

  on("process.run", async (_$: any, e: any) => {
    world.runs.push({ argv: e.argv, stdin: e.init?.stdin, cwd: e.init?.cwd })
    const isPlan = e.argv[2] === "classify-plan"
    if (!isPlan) return { value: { exitCode: 0, stdout: '{"outcome":"imported","reason":"ok"}\n', stderr: "" } }
    const stdout = typeof behaviour.plan === "string" ? behaviour.plan : JSON.stringify(behaviour.plan)
    return { value: { exitCode: behaviour.planExitCode ?? 0, stdout, stderr: "" } }
  })

  on("model.classify", async (_$: any, e: any) => {
    world.classifications.push({ text: e.text, labels: e.labels, model: e.options?.model })
    if (behaviour.holdUntilAllStarted !== undefined) {
      if (world.classifications.length === behaviour.holdUntilAllStarted) release()
      await gate
    }
    const attempt = (attempts.get(e.text) ?? 0) + 1
    attempts.set(e.text, attempt)
    const answer = behaviour.classify ? behaviour.classify(e.text, attempt) : "yes"
    if (answer instanceof Error) return { deny: "classification refused" }
    return { value: answer }
  })

  for (const event of FORBIDDEN_EVENTS) {
    on(event, async () => {
      world.forbidden.push(event)
      return { deny: "not allowed" }
    })
  }
  return world
}

async function proveCommand($: any): Promise<void> {
  await $.tool.call({ tool: "Bash", command: PROVE_COMMAND })
}

async function startTurn($: any): Promise<void> {
  await $.turn.start({ text: "continue", turnId: "turn-1" })
}

function importRuns(world: World) {
  return world.runs.filter((run) => run.argv[2] === "import")
}

const PLAN_ARGV = [BINARY, "systemone", "classify-plan", "--model", SESSION_MODEL, "--root", SESSION_CWD]

test("a prove command plans, classifies each call in order with its planned model, then imports", { timeoutMs: TEST_TIMEOUT_MS }, async ($, on) => {
  const calls = [call("q1", 0, "text one"), call("q1", 1, "text two"), call("q2", 0, "text three")]
  const world = stage(on, { plan: [item(calls)] })
  await proveCommand($)

  expect(world.runs[0].argv).toEqual(PLAN_ARGV)
  expect(world.runs[0].cwd).toBe(SESSION_CWD)
  expect(world.classifications).toEqual(
    calls.map((planned) => ({ text: planned.text, labels: planned.labels, model: planned.model })),
  )
  const imports = importRuns(world)
  expect(imports.length).toBe(1)
  expect(imports[0].argv).toEqual([
    BINARY, "systemone", "import", TICKET, "--nonce", NONCE, "--source", SOURCE, "--root", SESSION_CWD,
  ])
  expect(imports[0].cwd).toBe(SESSION_CWD)
  expect(JSON.parse(imports[0].stdin as string)).toEqual({
    cliVersion: CLI_VERSION,
    sessionModel: SESSION_MODEL,
    calls: calls.map((planned) => ({
      question: planned.question, i: planned.i, model: planned.model, label: "yes", attempts: 1,
    })),
  })
})

test("a turn start picks up pending tickets", { timeoutMs: TEST_TIMEOUT_MS }, async ($, on) => {
  const world = stage(on, { plan: [item([call("q1", 0, "turn text")])] })
  await startTurn($)
  expect(world.classifications.length).toBe(1)
  expect(importRuns(world).length).toBe(1)
})

test("a Bash command that is not a prove command does nothing", { timeoutMs: TEST_TIMEOUT_MS }, async ($, on) => {
  const world = stage(on, { plan: [item([call("q1", 0, "text")])] })
  await $.tool.call({ tool: "Bash", command: "ls " + BINARY + " prove" })
  await $.tool.call({ tool: "Bash", command: BINARY + " proveall" })
  expect(world.runs.length).toBe(0)
})

test("all calls of an item start before any resolves", { timeoutMs: TEST_TIMEOUT_MS }, async ($, on) => {
  const calls = [call("q1", 0, "a"), call("q1", 1, "b"), call("q2", 0, "c"), call("q2", 1, "d")]
  const world = stage(on, { plan: [item(calls)], holdUntilAllStarted: calls.length })
  await proveCommand($)
  expect(world.started()).toBe(calls.length)
  expect(importRuns(world).length).toBe(1)
})

test("a rejected call is retried once with the same arguments and keeps the second answer", { timeoutMs: TEST_TIMEOUT_MS }, async ($, on) => {
  const world = stage(on, {
    plan: [item([call("q1", 0, "flaky")])],
    classify: (_text, attempt) => (attempt === 1 ? new Error("rejected") : "no"),
  })
  await proveCommand($)
  expect(world.classifications.length).toBe(MAX_ATTEMPTS)
  expect(world.classifications[0]).toEqual(world.classifications[1])
  const body = JSON.parse(importRuns(world)[0].stdin as string)
  expect(body.calls[0].label).toBe("no")
  expect(body.calls[0].attempts).toBe(MAX_ATTEMPTS)
})

test("an undefined label twice is a null label after two attempts", { timeoutMs: TEST_TIMEOUT_MS }, async ($, on) => {
  const world = stage(on, { plan: [item([call("q1", 0, "silent")])], classify: () => undefined })
  await proveCommand($)
  expect(world.classifications.length).toBe(MAX_ATTEMPTS)
  const body = JSON.parse(importRuns(world)[0].stdin as string)
  expect(body.calls[0].label).toBe(null)
  expect(body.calls[0].attempts).toBe(MAX_ATTEMPTS)
})

test("a call rejected on its last attempt is reported as rejected, and one that returned no label is not", { timeoutMs: TEST_TIMEOUT_MS }, async ($, on) => {
  const rejectedWorld = stage(on, { plan: [item([call("q1", 0, "refused")])], classify: () => new Error("rejected") })
  await proveCommand($)
  const rejected = JSON.parse(importRuns(rejectedWorld)[0].stdin as string)
  expect(rejected.calls[0].label).toBe(null)
  expect(rejected.calls[0].attempts).toBe(MAX_ATTEMPTS)
  expect(rejected.calls[0].rejected).toBe(true)
})

test("a rejection followed by an undefined label is a missing vote, not a rejected call", { timeoutMs: TEST_TIMEOUT_MS }, async ($, on) => {
  const world = stage(on, {
    plan: [item([call("q1", 0, "mixed")])],
    classify: (_text, attempt) => (attempt === 1 ? new Error("rejected") : undefined),
  })
  await proveCommand($)
  const body = JSON.parse(importRuns(world)[0].stdin as string)
  expect(body.calls[0].label).toBe(null)
  expect(body.calls[0].rejected).toBe(undefined)
})

test("an empty plan makes no classify call and no import", { timeoutMs: TEST_TIMEOUT_MS }, async ($, on) => {
  const world = stage(on, { plan: [] })
  await proveCommand($)
  expect(world.classifications.length).toBe(0)
  expect(importRuns(world).length).toBe(0)
})

test("a failing plan command does nothing and does not throw", { timeoutMs: TEST_TIMEOUT_MS }, async ($, on) => {
  const world = stage(on, { plan: [item([call("q1", 0, "text")])], planExitCode: FAILURE_EXIT_CODE })
  await proveCommand($)
  expect(world.classifications.length).toBe(0)
  expect(importRuns(world).length).toBe(0)
})

test("an unparsable plan does nothing and does not throw", { timeoutMs: TEST_TIMEOUT_MS }, async ($, on) => {
  const world = stage(on, { plan: "not json" })
  await proveCommand($)
  expect(world.classifications.length).toBe(0)
  expect(importRuns(world).length).toBe(0)
})

test("each item is imported on its own with its own ticket", { timeoutMs: TEST_TIMEOUT_MS }, async ($, on) => {
  const world = stage(on, { plan: [item([call("q1", 0, "one")], "ticket-a"), item([call("q1", 0, "two")], "ticket-b")] })
  await proveCommand($)
  expect(importRuns(world).map((run) => run.argv[3])).toEqual(["ticket-a", "ticket-b"])
})

test("the nonce and the texts reach only the calls that need them and nothing else of the session", { timeoutMs: TEST_TIMEOUT_MS }, async ($, on) => {
  const secretText = "a ticket text that must stay with the classifier"
  const world = stage(on, { plan: [item([call("q1", 0, secretText)])] })
  await proveCommand($)
  expect(world.forbidden).toEqual([])
  const [planRun, importRun] = world.runs
  expect(JSON.stringify(planRun)).not.toContain(NONCE)
  expect(JSON.stringify(planRun)).not.toContain(secretText)
  expect(importRun.stdin as string).not.toContain(NONCE)
  expect(importRun.stdin as string).not.toContain(secretText)
  expect(importRun.argv.join(" ")).not.toContain(secretText)
})
