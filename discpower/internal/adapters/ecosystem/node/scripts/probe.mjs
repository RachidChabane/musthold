// Calls the bound operations of a Node project with the typed literals of a spec
// example and reports what each call returned or raised.
//
// usage: node probe.mjs <request.json> <response>
//
// The working directory is the project. Every module and tool is resolved from
// it, so the project's own TypeScript tooling is the one that loads its sources.
import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';
import Module, { createRequire } from 'node:module';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { isMainThread, Worker, workerData } from 'node:worker_threads';

const CALL_TIMEOUT_MS = 5000;
const WATCHDOG_INTERVAL_MS = 50;
const EXIT_FAILURE = 70;
const exitProcess = process.exit.bind(process);
const SIGKILL = 'SIGKILL';
const RANDOM_SEED = 0x9e3779b9;

const TARGET_SEPARATOR = ':';
const MEMBER_SEPARATOR = '.';
const OPERATION_SIGNATURES = 'signatures';

const RAISES_ARGUMENT = 'ArgumentError';
const RAISES_UNKNOWN_TARGET = 'UnknownTarget';
const RAISES_TIMEOUT = 'Timeout';
const RAISES_PROCESS_EXIT = 'ProcessExit';
const RAISES_UNNAMED = 'Error';

const HINT_BOOLEAN = 'boolean';
const HINT_INTEGER = 'integer';
const HINT_DECIMAL = 'decimal';
const HINT_STRING = 'string';
const HINT_DATETIME = 'datetime';
const HINT_UNKNOWN = 'unknown';

const TYPESCRIPT_EXTENSIONS = new Set(['.ts', '.tsx', '.mts', '.cts', '.jsx']);
const DECIMAL_LIBRARY_TYPES = new Set(['Decimal', 'Big', 'BigNumber']);
const MODULE_NOT_FOUND = 'MODULE_NOT_FOUND';
const SCRIPT_EXTENSION_FALLBACKS = new Map([
  ['.js', ['.ts', '.tsx']],
  ['.mjs', ['.mts']],
  ['.cjs', ['.cts']],
]);
const ECMASCRIPT_TARGET = 'ES2020';

// ---------------------------------------------------------------- watchdog

const STATE_IN_CALL = 0;
const STATE_CALL_NUMBER = 1;
const STATE_LENGTH = 2;

function watch({ state: buffer, responsePath }) {
  const state = new Int32Array(buffer);
  let watched = 0;
  let since = Date.now();
  setInterval(() => {
    const number = Atomics.load(state, STATE_CALL_NUMBER);
    if (number !== watched) {
      watched = number;
      since = Date.now();
    }
    const stuck = Atomics.load(state, STATE_IN_CALL) === 1 && Date.now() - since >= CALL_TIMEOUT_MS;
    if (stuck) {
      appendFileSync(responsePath, JSON.stringify(raised(RAISES_TIMEOUT)) + '\n');
      process.kill(process.pid, SIGKILL);
    }
  }, WATCHDOG_INTERVAL_MS);
}

// ----------------------------------------------------------------- literals

function literal(type, value) {
  return { type, value };
}

function returned(value) {
  return { kind: 'returns', ...literalOf(value) };
}

function raised(name) {
  return { kind: 'raises', value: name };
}

function plainDecimal(number) {
  const text = String(number);
  const match = /^(-?)(\d+)(?:\.(\d+))?e([+-]\d+)$/.exec(text);
  if (!match) {
    return text;
  }
  const [, sign, whole, fraction = '', exponent] = match;
  const digits = whole + fraction;
  const point = whole.length + Number(exponent);
  if (point <= 0) {
    return `${sign}0.${'0'.repeat(-point)}${digits}`;
  }
  if (point >= digits.length) {
    return sign + digits + '0'.repeat(point - digits.length);
  }
  return `${sign}${digits.slice(0, point)}.${digits.slice(point)}`;
}

function numberLiteral(number) {
  if (!Number.isFinite(number)) {
    return literal(HINT_STRING, String(number));
  }
  if (Number.isInteger(number)) {
    return literal(HINT_INTEGER, plainDecimal(Object.is(number, -0) ? 0 : number));
  }
  return literal(HINT_DECIMAL, plainDecimal(number));
}

function isoWithOffset(date) {
  const text = date.toISOString();
  return text.replace(/\.000Z$/, 'Z').replace(/Z$/, '+00:00');
}

function canonicalJson(value, ancestors = new Set()) {
  if (value === null) {
    return 'null';
  }
  switch (typeof value) {
    case 'boolean':
      return String(value);
    case 'number':
      return Number.isFinite(value) ? numberLiteral(value).value : 'null';
    case 'bigint':
      return JSON.stringify(value.toString());
    case 'string':
      return JSON.stringify(value);
    case 'undefined':
    case 'function':
    case 'symbol':
      return 'null';
    default:
      return canonicalObject(value, ancestors);
  }
}

function canonicalObject(value, ancestors) {
  if (ancestors.has(value)) {
    return JSON.stringify('[Circular]');
  }
  if (value instanceof Date) {
    return JSON.stringify(Number.isNaN(value.getTime()) ? null : isoWithOffset(value));
  }
  if (typeof value.toJSON === 'function') {
    return canonicalJson(value.toJSON(), ancestors);
  }
  const inside = new Set(ancestors).add(value);
  if (Array.isArray(value) || value instanceof Set) {
    return '[' + [...value].map((item) => canonicalJson(item, inside)).join(',') + ']';
  }
  if (value instanceof Map) {
    return '[' + [...value].map(([key, item]) => canonicalJson([key, item], inside)).join(',') + ']';
  }
  const members = Object.keys(value)
    .sort()
    .filter((key) => !['undefined', 'function', 'symbol'].includes(typeof value[key]))
    .map((key) => `${JSON.stringify(key)}:${canonicalJson(value[key], inside)}`);
  return '{' + members.join(',') + '}';
}

function literalOf(value) {
  switch (typeof value) {
    case 'undefined':
      return literal('null', 'null');
    case 'boolean':
      return literal('boolean', String(value));
    case 'number':
      return numberLiteral(value);
    case 'bigint':
      return literal(HINT_INTEGER, value.toString());
    case 'string':
      return literal(HINT_STRING, value);
    case 'symbol':
      return literal(HINT_STRING, String(value));
    default:
      return objectLiteral(value);
  }
}

function objectLiteral(value) {
  if (value === null) {
    return literal('null', 'null');
  }
  if (value instanceof Date) {
    return Number.isNaN(value.getTime()) ? literal(HINT_STRING, 'Invalid Date') : literal(HINT_DATETIME, isoWithOffset(value));
  }
  if (typeof value === 'function') {
    return literal(HINT_STRING, String(value));
  }
  return literal(HINT_STRING, canonicalJson(value));
}

function errorName(error) {
  if (error === null || error === undefined) {
    return RAISES_UNNAMED;
  }
  const constructorName = Object(error).constructor?.name;
  if (constructorName && constructorName !== 'Object') {
    return constructorName;
  }
  return typeof error.name === 'string' && error.name ? error.name : constructorName || RAISES_UNNAMED;
}

class ProcessExit extends Error {}

// ------------------------------------------------------------------ loading

const projectRequire = createRequire(path.join(process.cwd(), 'probe-anchor.js'));

function resolveInProject(specifier) {
  try {
    return projectRequire.resolve(specifier);
  } catch {
    return null;
  }
}

let typescriptCompilerInstalled = false;

function useTypeScriptCompiler(typescript) {
  if (typescriptCompilerInstalled) {
    return;
  }
  typescriptCompilerInstalled = true;
  const compile = (module, filename) => {
    const { outputText } = typescript.transpileModule(readFileSync(filename, 'utf8'), {
      fileName: filename,
      compilerOptions: {
        module: typescript.ModuleKind.CommonJS,
        target: typescript.ScriptTarget[ECMASCRIPT_TARGET],
        esModuleInterop: true,
        jsx: typescript.JsxEmit.ReactJSX,
      },
    });
    module._compile(outputText, filename);
  };
  for (const extension of TYPESCRIPT_EXTENSIONS) {
    Module._extensions[extension] = compile;
  }
  const resolveFilename = Module._resolveFilename;
  Module._resolveFilename = function resolveTypeScriptSibling(request, ...rest) {
    try {
      return resolveFilename.call(this, request, ...rest);
    } catch (error) {
      const extension = path.extname(request);
      for (const sibling of SCRIPT_EXTENSION_FALLBACKS.get(extension) ?? []) {
        try {
          return resolveFilename.call(this, request.slice(0, -extension.length) + sibling, ...rest);
        } catch {
          // try the next sibling
        }
      }
      throw error;
    }
  };
}

async function importWithTsx(file) {
  const api = resolveInProject('tsx/esm/api');
  if (api === null) {
    return null;
  }
  const { tsImport } = await import(pathToFileURL(api).href);
  return tsImport(pathToFileURL(file).href, pathToFileURL(path.join(process.cwd(), 'probe-anchor.js')).href);
}

function requireWithTsNode(file) {
  if (resolveInProject('ts-node') === null) {
    return null;
  }
  projectRequire('ts-node').register({ transpileOnly: true, compilerOptions: { module: 'commonjs' } });
  return projectRequire(file);
}

function requireWithTypeScript(file) {
  if (resolveInProject('typescript') === null) {
    return null;
  }
  useTypeScriptCompiler(projectRequire('typescript'));
  return projectRequire(file);
}

async function loadTypeScript(file) {
  const loaded = (await importWithTsx(file)) ?? requireWithTsNode(file) ?? requireWithTypeScript(file);
  if (loaded === null) {
    throw new Error(`no TypeScript loader (tsx, ts-node or typescript) is installed to load ${file}`);
  }
  return loaded;
}

const modules = new Map();

async function loadModule(file) {
  if (!modules.has(file)) {
    const absolute = path.resolve(process.cwd(), file);
    const loaded = TYPESCRIPT_EXTENSIONS.has(path.extname(absolute))
      ? await loadTypeScript(absolute)
      : await import(pathToFileURL(absolute).href);
    modules.set(file, loaded);
  }
  return modules.get(file);
}

function parseTarget(text) {
  const separator = text.lastIndexOf(TARGET_SEPARATOR);
  if (separator <= 0 || separator === text.length - 1) {
    throw new Error(`${text} is not file:function`);
  }
  return { text, file: text.slice(0, separator), path: text.slice(separator + 1).split(MEMBER_SEPARATOR) };
}

function exported(namespace, name) {
  for (const holder of [namespace, namespace?.default]) {
    if (holder !== null && holder !== undefined && holder[name] !== undefined) {
      return holder[name];
    }
  }
  return undefined;
}

// resolveTarget finds what a target names: a function, a static method, or a
// method of a class, which each call runs on a fresh instance made without
// arguments.
async function resolveTarget(target) {
  const namespace = await loadModule(target.file);
  const [head, ...steps] = target.path;
  let owner = namespace;
  let callable = exported(namespace, head);
  let onInstance = false;
  for (const step of steps) {
    owner = callable;
    callable = owner?.[step];
    onInstance = callable === undefined && typeof owner === 'function';
    if (onInstance) {
      callable = owner.prototype?.[step];
    }
  }
  if (typeof callable !== 'function') {
    return null;
  }
  const name = steps.at(-1);
  const run = onInstance ? (args) => new owner()[name](...args) : (args) => callable.apply(owner, args);
  return { callable, run };
}

// --------------------------------------------------------------- signatures

const NAME_BOUNDARY = /([a-z0-9])([A-Z])/g;

function snakeCase(name) {
  return name.replace(NAME_BOUNDARY, '$1_$2').toLowerCase();
}

function comparable(name) {
  return name.replaceAll('_', '').toLowerCase();
}

function hintOfDefaultText(text) {
  if (/^-?\d+$/.test(text)) {
    return HINT_INTEGER;
  }
  if (/^-?(\d+\.\d*|\.\d+)$/.test(text)) {
    return HINT_DECIMAL;
  }
  if (/^(['"`]).*\1$/s.test(text)) {
    return HINT_STRING;
  }
  return /^(true|false)$/.test(text) ? HINT_BOOLEAN : HINT_UNKNOWN;
}

function parameter(name, hint, { optional, nullable, bigint }) {
  return { name, key: comparable(name), hint, optional, nullable, bigint };
}

function splitTopLevel(text) {
  const parts = [];
  let depth = 0;
  let quote = null;
  let start = 0;
  for (let index = 0; index < text.length; index++) {
    const char = text[index];
    if (quote) {
      if (char === '\\') {
        index++;
      } else if (char === quote) {
        quote = null;
      }
    } else if ('\'"`'.includes(char)) {
      quote = char;
    } else if ('([{<'.includes(char)) {
      depth++;
    } else if (')]}>'.includes(char) && !(char === '>' && text[index - 1] === '=')) {
      depth--;
    } else if (char === ',' && depth === 0) {
      parts.push(text.slice(start, index));
      start = index + 1;
    }
  }
  parts.push(text.slice(start));
  return parts.map((part) => part.trim()).filter(Boolean);
}

function parameterListOf(source) {
  const arrow = /^(?:async\s+)?([A-Za-z_$][\w$]*)\s*=>/.exec(source);
  if (arrow) {
    return arrow[1];
  }
  const open = source.indexOf('(');
  let depth = 0;
  for (let index = open; index < source.length; index++) {
    depth += source[index] === '(' ? 1 : source[index] === ')' ? -1 : 0;
    if (depth === 0) {
      return source.slice(open + 1, index);
    }
  }
  return '';
}

// textualParameters reads the parameters of a function from its source, for a
// project without a TypeScript compiler: names and the hints of defaults only.
function textualParameters(callable) {
  const parameters = [];
  for (const part of splitTopLevel(parameterListOf(Function.prototype.toString.call(callable)))) {
    if (part.startsWith('...')) {
      continue;
    }
    const [declared, ...defaultText] = part.split('=');
    const name = declared.trim();
    const fallback = defaultText.join('=').trim();
    const named = /^[A-Za-z_$][\w$]*$/.test(name);
    parameters.push(
      parameter(named ? name : `arg${parameters.length}`, named ? hintOfDefaultText(fallback) : HINT_UNKNOWN, {
        optional: fallback !== '',
        nullable: null,
        bigint: false,
      }),
    );
  }
  return parameters;
}

function typedParameters(typescript, file, path_) {
  const text = readFileSync(path.resolve(process.cwd(), file), 'utf8');
  const source = typescript.createSourceFile(file, text, typescript.ScriptTarget.Latest, true, scriptKindOf(typescript, file));
  const declaration = findDeclaration(typescript, source, path_);
  return declaration ? declaration.parameters.flatMap((node, index) => typedParameter(typescript, node, index)) : null;
}

function scriptKindOf(typescript, file) {
  const kinds = new Map([
    ['.ts', typescript.ScriptKind.TS],
    ['.mts', typescript.ScriptKind.TS],
    ['.cts', typescript.ScriptKind.TS],
    ['.tsx', typescript.ScriptKind.TSX],
    ['.jsx', typescript.ScriptKind.JSX],
  ]);
  return kinds.get(path.extname(file)) ?? typescript.ScriptKind.JS;
}

function findDeclaration(typescript, source, path_) {
  const [head, name] = path_;
  const named = (node) => node.name && node.name.text === (path_.length === 1 ? head : name);
  let found = null;
  const visit = (node) => {
    if (found) {
      return;
    }
    if (path_.length === 1 && typescript.isFunctionDeclaration(node) && named(node)) {
      found = node;
    } else if (path_.length === 1 && typescript.isVariableDeclaration(node) && named(node) && isFunctionValue(typescript, node.initializer)) {
      found = node.initializer;
    } else if (path_.length === 2 && typescript.isClassDeclaration(node) && node.name?.text === head) {
      found = node.members.find((member_) => typescript.isMethodDeclaration(member_) && named(member_)) ?? null;
    }
    typescript.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

function isFunctionValue(typescript, node) {
  return node !== undefined && (typescript.isArrowFunction(node) || typescript.isFunctionExpression(node));
}

function typedParameter(typescript, node, index) {
  if (node.dotDotDotToken) {
    return [];
  }
  const named = typescript.isIdentifier(node.name);
  const type = node.type ? typeFacts(typescript, node.type) : { hint: HINT_UNKNOWN, nullable: null, bigint: false };
  const fromDefault = node.initializer ? hintOfDefaultText(node.initializer.getText()) : HINT_UNKNOWN;
  return [
    parameter(named ? node.name.text : `arg${index}`, named ? (type.hint === HINT_UNKNOWN ? fromDefault : type.hint) : HINT_UNKNOWN, {
      optional: Boolean(node.questionToken || node.initializer),
      nullable: type.nullable,
      bigint: type.bigint,
    }),
  ];
}

function typeFacts(typescript, node) {
  const { SyntaxKind } = typescript;
  const kinds = new Map([
    [SyntaxKind.NumberKeyword, HINT_DECIMAL],
    [SyntaxKind.BigIntKeyword, HINT_INTEGER],
    [SyntaxKind.StringKeyword, HINT_STRING],
    [SyntaxKind.BooleanKeyword, HINT_BOOLEAN],
    [SyntaxKind.TrueKeyword, HINT_BOOLEAN],
    [SyntaxKind.FalseKeyword, HINT_BOOLEAN],
  ]);
  if (typescript.isUnionTypeNode(node)) {
    return unionFacts(typescript, node);
  }
  if (typescript.isParenthesizedTypeNode(node)) {
    return typeFacts(typescript, node.type);
  }
  if (typescript.isLiteralTypeNode(node)) {
    return { hint: literalTypeHint(typescript, node.literal), nullable: null, bigint: false };
  }
  if (typescript.isTypeReferenceNode(node)) {
    return { hint: referenceHint(node.typeName.getText()), nullable: null, bigint: false };
  }
  return { hint: kinds.get(node.kind) ?? HINT_UNKNOWN, nullable: null, bigint: node.kind === SyntaxKind.BigIntKeyword };
}

function literalTypeHint(typescript, literalNode) {
  if (typescript.isNumericLiteral(literalNode)) {
    return hintOfDefaultText(literalNode.text);
  }
  if (typescript.isStringLiteral(literalNode)) {
    return HINT_STRING;
  }
  return literalNode.kind === typescript.SyntaxKind.TrueKeyword || literalNode.kind === typescript.SyntaxKind.FalseKeyword ? HINT_BOOLEAN : HINT_UNKNOWN;
}

function referenceHint(name) {
  if (name === 'Date') {
    return HINT_DATETIME;
  }
  return DECIMAL_LIBRARY_TYPES.has(name.split(MEMBER_SEPARATOR).at(-1)) ? HINT_DECIMAL : HINT_UNKNOWN;
}

function unionFacts(typescript, node) {
  const { SyntaxKind } = typescript;
  const members = node.types.filter((member_) => member_.kind !== SyntaxKind.UndefinedKeyword && !isNullType(typescript, member_));
  const acceptsUndefined = node.types.some((member_) => member_.kind === SyntaxKind.UndefinedKeyword);
  const acceptsNull = node.types.some((member_) => isNullType(typescript, member_));
  const nullable = acceptsNull ? 'null' : acceptsUndefined ? 'undefined' : null;
  const facts = members.map((member_) => typeFacts(typescript, member_));
  const agreed = facts.length > 0 && facts.every((fact) => fact.hint === facts[0].hint);
  return { hint: agreed ? facts[0].hint : HINT_UNKNOWN, nullable, bigint: agreed && facts[0].bigint };
}

function isNullType(typescript, node) {
  return typescript.isLiteralTypeNode(node) && node.literal.kind === typescript.SyntaxKind.NullKeyword;
}

function parametersOf(target, resolved) {
  const typescriptPath = resolveInProject('typescript');
  if (typescriptPath !== null) {
    const typed = typedParameters(projectRequire('typescript'), target.file, target.path);
    if (typed !== null) {
      return typed;
    }
  }
  return textualParameters(resolved.callable);
}

// ---------------------------------------------------------------- arguments

function convert(literalValue, expected) {
  switch (literalValue.type) {
    case 'null':
      return expected.nullable === 'undefined' ? undefined : null;
    case HINT_INTEGER:
      return convertInteger(literalValue.value, expected);
    case HINT_DECIMAL:
      return numberFrom(literalValue.value);
    case HINT_BOOLEAN:
      return literalValue.value === 'true';
    case HINT_STRING:
    case HINT_DATETIME:
      return expected.hint === HINT_DATETIME ? dateFrom(literalValue.value) : literalValue.value;
    default:
      throw new Error(`unknown literal type ${literalValue.type}`);
  }
}

function numberFrom(text) {
  const number = Number(text);
  if (Number.isNaN(number)) {
    throw new Error(`${text} is not a number`);
  }
  return number;
}

function convertInteger(text, expected) {
  const number = numberFrom(text);
  return expected.bigint || !Number.isSafeInteger(number) ? BigInt(text) : number;
}

function dateFrom(text) {
  const date = new Date(text);
  if (Number.isNaN(date.getTime())) {
    throw new Error(`${text} is not an ISO-8601 datetime`);
  }
  return date;
}

function argumentsOf(parameters, given) {
  const byKey = new Map();
  for (const [name, value] of Object.entries(given)) {
    if (byKey.has(comparable(name))) {
      throw new Error(`argument ${name} given twice`);
    }
    byKey.set(comparable(name), value);
  }
  const values = parameters.map((expected) => {
    const supplied = byKey.get(expected.key);
    byKey.delete(expected.key);
    if (supplied === undefined) {
      if (!expected.optional) {
        throw new Error(`argument ${expected.name} missing`);
      }
      return undefined;
    }
    return convert(supplied, expected);
  });
  if (byKey.size > 0) {
    throw new Error(`${byKey.size} unexpected arguments`);
  }
  return values;
}

// -------------------------------------------------------------------- calls

const cache = new Map();

async function describeTarget(text) {
  if (!cache.has(text)) {
    const target = parseTarget(text);
    const resolved = await resolveTarget(target);
    cache.set(text, resolved === null ? null : { resolved, parameters: parametersOf(target, resolved) });
  }
  return cache.get(text);
}

let current = null;

function raiseInCurrentCall(error) {
  current?.reject(error);
}

function runCall(state, entry, values) {
  return new Promise((resolve, reject) => {
    current = { reject };
    Atomics.add(state, STATE_CALL_NUMBER, 1);
    Atomics.store(state, STATE_IN_CALL, 1);
    Promise.resolve()
      .then(() => entry.resolved.run(values))
      .then(resolve, reject);
  }).finally(() => {
    Atomics.store(state, STATE_IN_CALL, 0);
    current = null;
  });
}

async function observe(state, call) {
  let entry;
  try {
    entry = await describeTarget(call.target);
  } catch (error) {
    throw new Error(`${call.target}: ${error.stack ?? error}`);
  }
  if (entry === null) {
    return raised(RAISES_UNKNOWN_TARGET);
  }
  let values;
  try {
    values = argumentsOf(entry.parameters, call.args);
  } catch {
    return raised(RAISES_ARGUMENT);
  }
  try {
    return returned(await runCall(state, entry, values));
  } catch (error) {
    return raised(errorName(error));
  }
}

async function signaturesOf(targets) {
  const signatures = [];
  for (const text of targets) {
    const entry = await describeTarget(text);
    if (entry === null) {
      throw new Error(`unknown target ${text}`);
    }
    signatures.push({ target: text, params: entry.parameters.map(({ name, hint }) => ({ name: snakeCase(name), hint })) });
  }
  return signatures;
}

function seedRandom() {
  let seed = RANDOM_SEED;
  Math.random = () => {
    seed = (seed + 0x6d2b79f5) | 0;
    let mixed = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    mixed = (mixed + Math.imul(mixed ^ (mixed >>> 7), 61 | mixed)) ^ mixed;
    return ((mixed ^ (mixed >>> 14)) >>> 0) / 4294967296;
  };
}

async function main(requestPath, responsePath) {
  const request = JSON.parse(readFileSync(requestPath, 'utf8'));
  if (request.op === OPERATION_SIGNATURES) {
    writeFileSync(responsePath, JSON.stringify({ signatures: await signaturesOf(request.targets) }));
    return;
  }
  const state = new Int32Array(new SharedArrayBuffer(STATE_LENGTH * Int32Array.BYTES_PER_ELEMENT));
  new Worker(new URL(import.meta.url), { workerData: { state: state.buffer, responsePath } });
  process.exit = (code) => {
    throw new ProcessExit(`exit ${code}`);
  };
  process.on('uncaughtException', raiseInCurrentCall);
  process.on('unhandledRejection', raiseInCurrentCall);
  seedRandom();
  for (const call of request.calls) {
    appendFileSync(responsePath, JSON.stringify(await observe(state, call)) + '\n');
  }
  exitProcess(0);
}

if (isMainThread) {
  main(process.argv[2], process.argv[3]).catch((error) => {
    process.stderr.write(`${error.stack ?? error}\n`);
    exitProcess(EXIT_FAILURE);
  });
} else {
  watch(workerData);
}
