import { describe, expect, it } from 'vitest';
import { discounted } from '../src/shipping';

describe('case', () => {
  it.each([
    [10, 200, 180],
    [50, 80, 40],
  ])('case', (percent, total, expected) => {
    expect(discounted(total, percent)).toBe(expected);
  });
});
