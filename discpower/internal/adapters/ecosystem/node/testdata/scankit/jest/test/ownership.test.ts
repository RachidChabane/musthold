import { audit, credit, late, route, tick } from '../src/ledger';

const log: string[] = [];
const state = { n: 0 };

describe('ownership', () => {
  beforeAll(() => {
    setTimeout(() => tick(state), 5);
  });

  afterEach(async () => {
    audit(log);
    route(false);
    await new Promise((resolve) => setTimeout(resolve, 20));
  });

  test('credits', () => {
    expect(credit(1, 2)).toBe(3);
  });

  test('routes urgent', () => {
    expect(route(true)).toBe('express');
  });

  test('waits past a timer', async () => {
    await new Promise((resolve) => setTimeout(resolve, 40));
    expect(credit(2, 2)).toBe(4);
  });

  test('leaves work behind', () => {
    setTimeout(() => late(state), 1);
    expect(credit(0, 0)).toBe(0);
  });

  test('loads lazily', async () => {
    const { lazyTotal } = await import('../src/lazy');
    expect(lazyTotal()).toBe(42);
  });

  test.concurrent('credits concurrently', async () => {
    expect(credit(5, 5)).toBe(10);
  });
});
