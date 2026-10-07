'use strict';

class Ledger {
  constructor() {
    this.entries = [];
  }

  record(amount) {
    this.entries.push(amount);
    return this.entries.length;
  }
}

function halve(amount) {
  return amount / 2;
}

function refuses() {
  throw new RangeError('out of range');
}

module.exports = { Ledger, halve, refuses, answer: 42 };
