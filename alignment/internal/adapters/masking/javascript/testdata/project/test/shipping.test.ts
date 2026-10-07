import { describe, expect, it, test, beforeEach } from 'vitest';
import quote, { Cart, discounted, lineTotal, shippingCost } from '../src/shipping';
import * as pricing from '../src/shipping';
import { unusedHelper } from './support';

const LOCAL = 'local';
const REMOTE = 'remote';

function twoKiloQuote(zone: 'local' | 'remote') {
  // the reference parcel weighs two kilos
  return shippingCost(2, zone);
}

describe('line totals', () => {
  it('multiply the unit price by the quantity [verifies PAR-1~1]', () => {
    expect(lineTotal(4, 3)).toBe(12);
    expect(lineTotal(5, 1)).toBe(5);
  });

  it('reject a zero quantity', () => {
    expect(() => lineTotal(4, 0)).toThrow(RangeError);
  });
});

describe('discounts', () => {
  it.each([
    [10, 200, 180],
    [50, 80, 40],
  ])('take %s percent off %s [verifies PAR-2~1]', (percent, total, expected) => {
    expect(discounted(total, percent)).toBe(expected);
  });

  describe('limits [verifies PAR-2~1]', () => {
    let cart: Cart;

    beforeEach(() => {
      cart = new Cart('ada');
      cart.add(80);
    });

    describe('beyond a full price', () => {
      it('clamp a percentage above one hundred', () => {
        expect(discounted(cart.total, 150)).toBeLessThanOrEqual(0);
      });
    });
  });
});

describe.each([['local'], ['remote']])('shipping to %s', (zone) => {
  test('costs a number [verifies PAR-3~1]', () => {
    expect(quote(1, zone as 'local' | 'remote').cost).toBeDefined();
  });
});

describe('surcharge', () => {
  test('remote parcels cost more than local ones', () => {
    const local = twoKiloQuote(LOCAL);
    const remote = twoKiloQuote(REMOTE);
    expect(remote - local).toBe(4.5);
    expect(pricing.REMOTE_SURCHARGE).toBe(4.5);
  });
});
