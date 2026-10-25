import { quietLevel } from '../src/levels';

test('quiet level lowers loud levels', () => {
  expect(quietLevel(50)).toBe(40);
});

test('quiet level silences low levels', () => {
  expect(quietLevel(20)).toBe(0);
});
