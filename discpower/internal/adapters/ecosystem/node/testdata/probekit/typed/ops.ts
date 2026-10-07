import { clampTo } from './limits.js';

export enum Tier {
  Basic = 'basic',
  Premium = 'premium',
}

export class Account {
  constructor(private readonly tier: Tier = Tier.Basic) {}

  fee(amount: number): number {
    return this.tier === Tier.Premium ? clampTo(amount / 2) : clampTo(amount);
  }
}

export function signature(
  percent: number,
  name: string,
  active: boolean,
  when: Date,
  count: bigint,
  maybe: number | null,
  later?: string,
  label: 'a' | 'b' = 'a',
  ratio = 0.5,
  retries = 3,
  ...rest: number[]
): string {
  return [percent, name, active, when.toISOString(), count, maybe, later, label, ratio, retries, rest.length].join('|');
}

export function tierOf(name: string): Tier {
  return name === 'premium' ? Tier.Premium : Tier.Basic;
}

export function shifted(when: Date, days: number): Date {
  return new Date(when.getTime() + days * 86400000);
}

export function total(amount: number, tax: number | undefined): number {
  return amount + (tax ?? 0);
}
