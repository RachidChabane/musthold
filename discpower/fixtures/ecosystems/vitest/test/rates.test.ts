import { describe, expect, it } from 'vitest';
import { discounted, FREE_SHIPPING_FROM, lineTotal, SHIPPING_FEE, shippingFee } from '../src/rates';

describe('line totals', () => {
  it('multiply the unit price by the quantity [verifies PAR-1~1]', () => {
    expect(lineTotal(4, 3)).toBe(12);
  });

  it('are zero for no units', () => {
    expect(lineTotal(7, 0)).toBe(0);
  });
});

describe('discounts', () => {
  it.each([
    [10, 200, 180],
    [50, 80, 40],
    [0, 19.99, 19.99],
  ])('take %s percent off %s [verifies PAR-2~1]', (percent, total, expected) => {
    expect(discounted(total, percent)).toBe(expected);
  });

  describe('limits [verifies PAR-2~1]', () => {
    it('clamp a percentage above one hundred', () => {
      expect(discounted(80, 150)).toBe(0);
    });

    describe('below zero', () => {
      it('ignore a negative percentage', () => {
        expect(discounted(80, -5)).toBe(80);
      });
    });
  });
});

describe('shipping', () => {
  it('is waived from fifty [verifies PAR-3~1]', () => {
    expect(shippingFee(FREE_SHIPPING_FROM)).toBe(0);
    expect(shippingFee(FREE_SHIPPING_FROM - 0.01)).toBe(SHIPPING_FEE);
  });
});
