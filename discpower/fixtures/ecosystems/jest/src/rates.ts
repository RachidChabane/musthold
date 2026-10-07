export const FREE_SHIPPING_FROM = 50;
export const SHIPPING_FEE = 4.5;
export const MAX_PERCENT = 100;

export function lineTotal(unitPrice: number, quantity: number): number {
  return unitPrice * quantity;
}

export function clampPercent(percent: number): number {
  return Math.max(0, Math.min(MAX_PERCENT, percent));
}

export function discounted(total: number, percent: number): number {
  const kept = MAX_PERCENT - clampPercent(percent);
  return Math.round(((total * kept) / MAX_PERCENT) * 100) / 100;
}

export function shippingFee(total: number): number {
  if (total >= FREE_SHIPPING_FROM) {
    return 0;
  }
  return SHIPPING_FEE;
}
