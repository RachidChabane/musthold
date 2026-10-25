# Cinema tickets

The box office prices one ticket for one screening from the buyer's age, in
whole years. Prices are whole euros.

## Prices

- **BOX-1~1**: WHEN the buyer is 12 to 64 years old THE SYSTEM SHALL charge 12.
  - Example: ticket_price(age=30) => 12
- **BOX-2~1**: WHEN the buyer is 65 or older THE SYSTEM SHALL charge 8.
  - Example: ticket_price(age=65) => 8
- **BOX-3~1**: WHEN the buyer is 3 to 11 years old THE SYSTEM SHALL charge 6.
  - Example: ticket_price(age=7) => 6
- **BOX-4~1**: WHEN the buyer is younger than 3 THE SYSTEM SHALL charge nothing.
  - Example: ticket_price(age=2) => 0
