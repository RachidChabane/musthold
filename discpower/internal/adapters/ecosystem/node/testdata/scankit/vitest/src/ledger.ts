export function credit(balance: number, amount: number): number {
  return balance + amount;
}

export function route(urgent: boolean): string {
  return urgent ? express() : standard();
}

export function express(): string {
  return 'express';
}

export function standard(): string {
  return 'standard';
}

export function tick(state: { n: number }): void {
  state.n += 1;
}

export function late(state: { n: number }): void {
  state.n += 100;
}

export function audit(log: string[]): number {
  return log.length;
}
