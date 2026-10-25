import { quote } from '../src/rates';

test.each([
  [0, '0%'],
  [20, '20%'],
  [100, '100%'],
])('quote of %i is %s', (percent, expected) => {
  expect(quote(percent)).toBe(expected);
});

test.each([-1, 101])('quote of %i is refused', (percent) => {
  expect(quote(percent)).toBe('no quote');
});
