import { describe, expect, it } from 'vitest';
import { applyDiscount, invoiceTotal, lineAmount } from '../src/totals';

describe('line amounts', () => {
  it('multiply the price by the quantity [verifies INV-1~1]', () => {
    expect(lineAmount(4, 3)).toBe(12);
  });
});

describe('invoice totals', () => {
  it('add the lines [verifies INV-1~1]', () => {
    expect(invoiceTotal([[4, 3], [5, 1]])).toBe(21.25);
  });
});

describe('discounts', () => {
  it('apply the welcome code to the total', () => {
    expect(applyDiscount(10, 'WELCOME')).toBe(9);
  });
});
