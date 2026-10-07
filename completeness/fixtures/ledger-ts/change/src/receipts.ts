export const receiptLine = (label: string, amount: string): string =>
  `${label.padEnd(20)}${amount.padStart(10)}`;
