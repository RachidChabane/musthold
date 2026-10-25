import { credit, tick } from '../src/ledger';

describe('fake timers set before the test', () => {
  const state = { n: 0 };

  beforeAll(() => {
    jest.useFakeTimers();
    setTimeout(() => tick(state), 100);
  });

  afterAll(() => {
    jest.useRealTimers();
  });

  test('advances a timer it did not set', () => {
    jest.advanceTimersByTime(200);
    expect(credit(state.n, 1)).toBe(2);
  });
});

describe('fake timers of the test', () => {
  test('advances its own timer', () => {
    jest.useFakeTimers();
    const state = { n: 0 };
    setTimeout(() => tick(state), 100);
    jest.advanceTimersByTime(200);
    jest.useRealTimers();
    expect(state.n).toBe(1);
  });
});
