import { formatAmount } from '../src/format';

test('format pads the cents', () => {
  expect(formatAmount(1205, 'USD')).toBe('12.05 USD');
});

test('format defaults to the euro', () => {
  expect(formatAmount(1250)).toBe('12.50 EUR');
});

test('format accepts any amount', () => {
  for (const cents of [0, 7, 1999]) {
    formatAmount(cents);
  }
});
