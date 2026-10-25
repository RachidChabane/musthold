export const LOUD_FROM = 40;
export const MUTED_DROP = 10;

const muted = new Set<string>();

export function mute(channel: string): void {
  muted.add(channel);
}

export function isMuted(channel: string): boolean {
  return muted.has(channel);
}

export function route(channel: string, level: number): number {
  if (muted.has(channel)) {
    return quietLevel(level);
  }
  return level;
}

export function quietLevel(level: number): number {
  if (level >= LOUD_FROM) {
    return level - MUTED_DROP;
  }
  return 0;
}
