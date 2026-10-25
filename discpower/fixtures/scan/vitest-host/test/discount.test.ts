import { expect, test } from 'vitest';
import { discount } from '../src/totals';

test('discount takes the percent off', () => {
  expect(discount(200, 10)).toBe(180);
});

test('discount accepts any percent', () => {
  for (const percent of [0, 150]) {
    discount(200, percent);
  }
});
