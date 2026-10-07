export const TAX_RATE = 0.25;

export function lineAmount(price: number, quantity: number): number {
  if (quantity < 0) {
    throw new RangeError('quantity must not be negative');
  }
  return price * quantity;
}

export function invoiceTotal(lines: Array<[number, number]>): number {
  const amounts = lines.map(([price, quantity]) => lineAmount(price, quantity));
  const net = amounts.reduce((sum, amount) => sum + amount, 0);
  return Math.round(net * (1 + TAX_RATE) * 100) / 100;
}

export function applyDiscount(total: number, code: string): number {
  if (code === 'WELCOME') {
    return total * 0.9;
  }
  return total;
}

/** Renders a total for display. */
export function formatTotal(total: number): string {
  return `${total.toFixed(2)} EUR`;
}
