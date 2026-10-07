export class Cart {
  readonly owner: string;
  static created;
  constructor(owner: string);
  add(price: number): void;
  get total(): number;
}

export const discounted = (total: number, percent: number): number => ...;
