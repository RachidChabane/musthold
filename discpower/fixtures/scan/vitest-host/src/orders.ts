export function orderTotal(lines: Array<[number, number]>): number {
  let total = 0;
  for (const [unitPrice, quantity] of lines) {
    total += unitPrice * quantity;
  }
  return total;
}
