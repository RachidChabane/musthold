import { afterAll, beforeAll, describe, expect, test, vi } from 'vitest';
import { credit, tick } from '../src/ledger';

describe('fake timers set before the test', () => {
  const state = { n: 0 };

  beforeAll(() => {
    vi.useFakeTimers();
    setTimeout(() => tick(state), 100);
  });

  afterAll(() => {
    vi.useRealTimers();
  });

  test('advances a timer it did not set', () => {
    vi.advanceTimersByTime(200);
    expect(credit(state.n, 1)).toBe(2);
  });
});

describe('fake timers of the test', () => {
  test('advances its own timer', () => {
    vi.useFakeTimers();
    const state = { n: 0 };
    setTimeout(() => tick(state), 100);
    vi.advanceTimersByTime(200);
    vi.useRealTimers();
    expect(state.n).toBe(1);
  });
});
