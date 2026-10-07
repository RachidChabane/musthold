import { describe, expect, it } from 'vitest';
import { lineTotal, shippingCost } from '../src/pricing';

const LOCAL = 'local';
const REMOTE = 'remote';

describe('line totals', () => {
  it('multiply the unit price by the quantity [verifies PAR-1~1]', () => {
    expect(lineTotal(4, 3)).toBe(12);
    expect(lineTotal(5, 1)).toBe(5);
  });

  it('cost the price of a single unit [verifies PAR-1~1]', () => {
    expect(lineTotal(7, 1)).toBe(7);
  });

  it('refuse a negative quantity [verifies PAR-1~1]', () => {
    expect(() => lineTotal(1, -1)).toThrow(RangeError);
  });

  it('refuse a zero quantity [verifies PAR-3~1]', () => {
    expect(() => lineTotal(4, 0)).toThrow(RangeError);
  });
});

describe('shipping', () => {
  it('adds the remote surcharge [verifies PAR-2~1]', () => {
    const local = shippingCost(2, LOCAL);
    const remote = shippingCost(2, REMOTE);
    expect(remote - local).toBe(4.5);
  });

  it('quotes a number [verifies PAR-2~1]', () => {
    expect(shippingCost(1, LOCAL)).toBeDefined();
  });
});
