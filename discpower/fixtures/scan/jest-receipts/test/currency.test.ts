import { parseCurrency } from '../src/format';

test('parse currency upper-cases the code', () => {
  expect(parseCurrency('usd')).toBe('USD');
});
