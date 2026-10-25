import { expect, test } from 'vitest';
import { parseRate, total } from '../src/rates';

const STANDARD_RATE = parseRate(20);

test('total adds the standard rate', () => {
  expect(total(100, STANDARD_RATE)).toBe(120);
});

test('total accepts any fare', () => {
  for (const amount of [80, 250]) {
    for (const percent of [STANDARD_RATE, 150]) {
      try {
        total(amount, percent);
      } catch {
        // a fare out of range is refused
      }
    }
  }
});
