import { archive } from '../src/archive';

describe('a batch', () => {
  let batch: number[] = [];

  beforeEach(() => {
    batch = [3, 4];
  });

  afterEach(() => {
    archive(batch);
  });

  test('holds two entries', () => {
    expect(batch).toHaveLength(2);
  });
});

test('archive sums the positive entries', () => {
  expect(archive([3, -1, 4])).toEqual([7, 2]);
});
