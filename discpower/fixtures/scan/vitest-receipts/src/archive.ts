export const ARCHIVE_LIMIT = 50;

export function archive(entries: number[]): [number, number] {
  let kept = entries.filter((entry) => entry > 0);
  if (kept.length > ARCHIVE_LIMIT) {
    kept = kept.slice(-ARCHIVE_LIMIT);
  }
  const total = kept.reduce((sum, entry) => sum + entry, 0);
  return [total, kept.length];
}
