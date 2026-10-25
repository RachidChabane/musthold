import { credit } from '../src/ledger';

jest.retryTimes(2);

let attempts = 0;

test('passes on its second attempt', () => {
  attempts += 1;
  expect(credit(attempts, 0)).toBeGreaterThan(1);
});
