# Parcel pricing

The parcel service prices an order: line totals, discounts and shipping.

## Pricing

- **PAR-1~1**: WHEN an order has line items THE SYSTEM SHALL total each line as
  its unit price times its quantity.
  - Example: line_total(unit_price=4.50, quantity=3) => 13.50
- **PAR-2~1**: WHEN a discount percentage is applied THE SYSTEM SHALL reduce the
  total by that percentage, limited to the range 0 to 100.
  - Example: discounted(total=200.00, percent=10) => 180.00
  - Example: discounted(total=80.00, percent=150) => 0.00

## Shipping

- **PAR-3~1**: WHEN an order total reaches 50 THE SYSTEM SHALL waive the
  shipping fee.
