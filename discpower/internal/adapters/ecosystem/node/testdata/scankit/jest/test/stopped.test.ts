import { loaded } from '../src/broken';

test('reads the archive', () => {
  expect(loaded).toBe(true);
});
