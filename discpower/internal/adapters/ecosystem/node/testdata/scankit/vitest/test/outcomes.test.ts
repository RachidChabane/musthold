import { afterEach, beforeAll, describe, expect, test } from 'vitest';
import { credit } from '../src/ledger';

describe('a failing beforeAll', () => {
  beforeAll(() => {
    throw new Error('no ledger');
  });

  test('never runs', () => {
    expect(credit(1, 1)).toBe(2);
  });
});

describe('a failing afterEach', () => {
  afterEach(() => {
    throw new Error('no cleanup');
  });

  test('passes its body', () => {
    expect(credit(1, 1)).toBe(2);
  });
});

test('fails', () => {
  expect(credit(1, 1)).toBe(3);
});

test('twin', () => {
  expect(credit(0, 1)).toBe(1);
});

test('twin', () => {
  expect(credit(1, 0)).toBe(1);
});

test('Case', () => {
  expect(credit(2, 0)).toBe(2);
});

test('case', () => {
  expect(credit(0, 2)).toBe(2);
});
