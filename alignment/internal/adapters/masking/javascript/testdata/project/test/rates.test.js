const { surcharge } = require('../src/rates');
const rates = require('../src/rates');

/* heavy parcels start above twenty kilos */
test('heavy parcels pay the fee [verifies PAR-2~1]', () => {
  expect(surcharge(25)).toBe(rates.FEE);
});

test('light parcels pay nothing', () => {
  expect(surcharge(1)).toBe(0);
});
