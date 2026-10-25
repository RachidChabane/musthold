export const MIN_PERCENT = 0;
export const MAX_PERCENT = 100;
export const PERCENT_BASE = 100;

export function parseRate(percent: number): number {
  if (percent < MIN_PERCENT || percent > MAX_PERCENT) {
    throw new RangeError('percentage out of range');
  }
  return percent;
}

export function total(amount: number, percent: number): number {
  const rate = parseRate(percent);
  return amount + Math.floor((amount * rate) / PERCENT_BASE);
}

export function quote(percent: number): string {
  try {
    return `${parseRate(percent)}%`;
  } catch {
    return 'no quote';
  }
}
