import { describe, expect, it } from 'vitest';
import { lineTotal } from '../src/shipping';

describe('case', () => {
  it('case', () => {
    expect(lineTotal(4, 3)).toBe(12);
    expect(lineTotal(5, 1)).toBe(5);
  });
});
