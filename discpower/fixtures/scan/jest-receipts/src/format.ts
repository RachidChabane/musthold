export const EURO = 'EUR';
export const CENTS = 100;
export const PAD_BELOW = 10;
export const CODE_LENGTH = 3;

export function formatAmount(cents: number, currency = ''): string {
  if (!currency) {
    currency = EURO;
  }
  const units = Math.floor(cents / CENTS);
  const rest = cents % CENTS;
  if (rest < PAD_BELOW) {
    return `${units}.0${rest} ${currency}`;
  }
  return `${units}.${rest} ${currency}`;
}

export function parseCurrency(code: string): string {
  if (code.length !== CODE_LENGTH) {
    throw new RangeError('a currency has three letters');
  }
  return code.toUpperCase();
}
