# tally

Computes the totals of a shop's orders: order totals, discounts and shipping.

## Orders

`OrderTotal(lines)` adds up an order given as lines of a unit price and a quantity.
The total of an order is the sum of its line totals.

## Discounts

`Discount(total, percent)` takes a percentage off a total.
A discount of a hundred percent or more makes the order free.

## Shipping

Orders of forty or more ship free.
Smaller orders pay a flat fee of five.

## Development

    go test ./...
