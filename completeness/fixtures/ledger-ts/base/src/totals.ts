export const TAX_RATE = 0.2;

export function lineAmount(price: number, quantity: number): number {
  return price * quantity;
}

export function invoiceTotal(lines: Array<[number, number]>): number {
  const amounts = lines.map(([price, quantity]) => lineAmount(price, quantity));
  return amounts.reduce((sum, amount) => sum + amount, 0);
}

export function formatTotal(total: number): string {
  return `${total.toFixed(2)} EUR`;
}
