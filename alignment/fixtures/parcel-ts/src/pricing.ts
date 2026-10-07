export const BASE_RATE = 5;
export const PER_KILO = 2.5;
export const REMOTE_SURCHARGE = 4.5;

export function lineTotal(unitPrice: number, quantity: number): number {
  if (quantity <= 0) {
    throw new RangeError('quantity must be positive');
  }
  return unitPrice * quantity;
}

export function shippingCost(weight: number, zone: string): number {
  const cost = BASE_RATE + weight * PER_KILO;
  return zone === 'remote' ? cost + REMOTE_SURCHARGE : cost;
}
