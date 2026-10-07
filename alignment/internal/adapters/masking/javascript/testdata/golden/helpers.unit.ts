import { describe, expect, test } from 'vitest';
import { shippingCost } from '../src/shipping';
import * as pricing from '../src/shipping';

const helper1 = 'local';

const helper2 = 'remote';

function helper3(zone: 'local' | 'remote') {
  return shippingCost(2, zone);
}

describe('case', () => {
  test('case', () => {
    const local = helper3(helper1);
    const remote = helper3(helper2);
    expect(remote - local).toBe(4.5);
    expect(pricing.REMOTE_SURCHARGE).toBe(4.5);
  });
});
