const { surcharge: helper1 } = require('../src/rates');

const helper2 = require('../src/rates');

test('case', () => {
  expect(helper1(25)).toBe(helper2.FEE);
});
