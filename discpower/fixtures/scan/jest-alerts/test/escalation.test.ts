import { escalate } from '../src/escalation';

test('escalate adds a share per repeat', () => {
  expect(escalate(200, 10)).toBe(220);
});

test('escalate accepts any repeat', () => {
  for (const repeats of [0, 150]) {
    escalate(200, repeats);
  }
});
