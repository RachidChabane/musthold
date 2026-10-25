export const CRITICAL = 100;
export const MAX_REPEATS = 100;
export const STEP_DIVISOR = 100;

export function escalate(level: number, repeats: number): number {
  if (repeats <= 0) {
    return level;
  }
  if (repeats >= MAX_REPEATS) {
    return CRITICAL;
  }
  return level + Math.floor((level * repeats) / STEP_DIVISOR);
}
