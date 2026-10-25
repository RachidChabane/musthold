export const FREE_SHIPPING_FROM = 50;
export const SHIPPING_FEE = 5;
export const FULL_PERCENT = 100;

export function lineTotal(unitPrice: number, quantity: number): number {
  return unitPrice * quantity;
}

export function discount(total: number, percent: number): number {
  if (percent <= 0) {
    return total;
  }
  if (percent >= FULL_PERCENT) {
    return 0;
  }
  return total - Math.floor((total * percent) / FULL_PERCENT);
}

export function shipping(total: number): number {
  if (total >= FREE_SHIPPING_FROM) {
    return 0;
  }
  return SHIPPING_FEE;
}
