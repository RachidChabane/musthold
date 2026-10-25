import { credit, tick } from '../src/ledger';

describe('legacy fake timers set before the test', () => {
  const state = { n: 0 };

  beforeAll(() => {
    jest.useFakeTimers({ legacyFakeTimers: true });
    setTimeout(() => tick(state), 100);
  });

  afterAll(() => {
    jest.useRealTimers();
  });

  test('advances a legacy timer it did not set', () => {
    jest.advanceTimersByTime(200);
    expect(credit(state.n, 1)).toBe(2);
  });
});
