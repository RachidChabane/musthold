import { lineTotal, shipping } from '../src/totals';

test('line total multiplies price by quantity', () => {
  expect(lineTotal(4, 3)).toBe(12);
});

test.each([
  [50, 0],
  [49, 5],
])('shipping of %i is %i', (total, fee) => {
  expect(shipping(total)).toBe(fee);
});
