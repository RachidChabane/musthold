export const LIMIT = 10;

export function lineTotal(unitPrice, quantity) {
  return unitPrice * quantity;
}

export function describeOrder(count = 1, label = 'order', ratio = 0.5, urgent = true, ...extras) {
  return `${label}:${count}:${ratio}:${urgent}:${extras.length}`;
}

export const scaled = (amount, factor = 2) => amount * factor;

export function exactSum(first, second) {
  return first + second;
}

export function tiny() {
  return 1e-7;
}

export function huge() {
  return 1e21;
}

export function nothing() {}

export function undecided() {
  return undefined;
}

export function asObject(first, second) {
  return { second, first, nested: { z: 1, y: [1, { b: 2, a: 1 }] }, skipped: undefined, tags: new Set(['x']) };
}

export function asDate(iso) {
  return new Date(iso);
}

export function nextDay(date) {
  return new Date(date.getTime() + 24 * 60 * 60 * 1000);
}

export function wide(count) {
  return BigInt(count) * 2n;
}

export function typeOf(value) {
  return typeof value;
}

export function nowAtNoon() {
  return new Date(2026, 2, 1, 12, 0, 0, 250);
}

export function luck() {
  return Math.random();
}

export async function asyncDouble(count) {
  await Promise.resolve();
  return count * 2;
}

export class ValidationError extends Error {}

export function failsWithTypeError() {
  throw new TypeError('bad type');
}

export function failsWithCustomError() {
  throw new ValidationError('invalid');
}

export function failsWithString() {
  throw 'refused';
}

export async function failsLater() {
  await Promise.resolve();
  throw new RangeError('late');
}

export function failsFromTimer() {
  return new Promise(() => {
    setTimeout(() => {
      throw new SyntaxError('from a timer');
    }, 1);
  });
}

export function exits() {
  process.exit(3);
}

export function crashes() {
  process.kill(process.pid, 'SIGKILL');
}

export function spins() {
  for (;;) {
    // runs until the process is killed
  }
}

export function hangs() {
  return new Promise(() => {});
}

export class Meter {
  constructor() {
    this.count = 0;
  }

  bump(by) {
    this.count += by;
    return this.count;
  }

  static from(start) {
    return start + 100;
  }
}
