import { expect, test } from 'vitest';
import { isMuted, mute, route } from '../src/levels';

test('mute silences the ops channel', () => {
  mute('ops');
  expect(isMuted('ops')).toBe(true);
});

test('route never raises a level', () => {
  expect(route('ops', 50)).toBeLessThanOrEqual(50);
});
