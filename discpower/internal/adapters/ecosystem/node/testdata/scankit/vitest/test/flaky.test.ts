import { expect, test } from 'vitest';
import { credit } from '../src/ledger';

let attempts = 0;

test('passes on its second attempt', { retry: 2 }, () => {
  attempts += 1;
  expect(credit(attempts, 0)).toBeGreaterThan(1);
});
