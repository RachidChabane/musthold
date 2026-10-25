import { beforeEach, describe, expect, test } from 'vitest';
import { credit, express } from '../src/ledger';

describe('setup', () => {
  beforeEach(() => {
    credit(9, 9);
  });

  test('reads after setup', () => {
    expect(express()).toBe('express');
  });
});
