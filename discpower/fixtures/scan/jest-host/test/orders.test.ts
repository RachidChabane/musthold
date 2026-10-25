import { orderTotal } from '../src/orders';

test('order total adds up the lines', () => {
  orderTotal([
    [4, 3],
    [2, 5],
  ]);
});
