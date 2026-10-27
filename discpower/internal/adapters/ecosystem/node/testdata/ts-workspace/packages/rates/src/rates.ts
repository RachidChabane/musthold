export const FULL_PERCENT = 100;

export function discount(total: number, percent: number): number {
  if (percent <= 0) return total;
  if (percent >= FULL_PERCENT) return 0;
  return total - (total * percent) / FULL_PERCENT;
}
