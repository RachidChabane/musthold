import { describe, expect, it, beforeEach } from 'vitest';
import { Cart, discounted } from '../src/shipping';

describe('case', () => {
  describe('case', () => {
    let helper1: Cart;

    beforeEach(() => {
      helper1 = new Cart('ada');
      helper1.add(80);
    });

    describe('case', () => {
      it('case', () => {
        expect(discounted(helper1.total, 150)).toBeLessThanOrEqual(0);
      });
    });
  });
});
