const FEE = 3;
const HEAVY_FROM = 20;

function surcharge(weight) {
  return weight > HEAVY_FROM ? FEE : 0;
}

module.exports = { surcharge, FEE };
