# invoice

Computes the totals of a shop's invoices: orders, discounts and shipping.

## Orders

`orderTotal(lines)` adds up an order given as `[unitPrice, quantity]` lines.
The total of an order is the sum of its line totals.

## Discounts

`discount(total, percent)` takes a percentage off a total.
A discount of a hundred percent or more makes the order free.

## Shipping

Orders of forty or more ship free.
Smaller orders pay a flat fee of five.

## Development

    npx vitest run
