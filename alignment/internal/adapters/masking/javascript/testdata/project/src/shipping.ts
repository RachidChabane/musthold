export const BASE_RATE = 5;
export const PER_KILO = 2.5;
export const REMOTE_SURCHARGE = 4.5;

export type Zone = 'local' | 'remote';

export interface Quote {
  cost: number;
  zone: Zone;
}

export function lineTotal(unitPrice: number, quantity: number): number {
  if (quantity <= 0) {
    throw new RangeError('quantity must be positive');
  }
  return unitPrice * quantity;
}

export function shippingCost(weight: number, zone: Zone): number {
  const cost = BASE_RATE + weight * PER_KILO;
  return zone === 'remote' ? cost + REMOTE_SURCHARGE : cost;
}

export const discounted = (total: number, percent: number): number => {
  return total - (total * percent) / 100;
};

export class Cart {
  private items: number[] = [];
  readonly owner: string;
  static created = 0;

  constructor(owner: string) {
    this.owner = owner;
  }

  add(price: number): void {
    this.items.push(price);
  }

  get total(): number {
    return this.items.reduce((sum, price) => sum + price, 0);
  }

  #audit(): void {}
}

export default function quote(weight: number, zone: Zone): Quote {
  return { cost: shippingCost(weight, zone), zone };
}
