import { describe, expect, test } from 'vitest';
import quote from '../src/shipping';

describe.each([['local'], ['remote']])('case', (zone) => {
  test('case', () => {
    expect(quote(1, zone as 'local' | 'remote').cost).toBeDefined();
  });
});
