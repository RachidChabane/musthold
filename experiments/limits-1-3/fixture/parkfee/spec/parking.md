# Car park tariff

The tariff applies to a single stay, from the entry time printed on the ticket to
the exit time at the barrier. Amounts are in euros. A started hour or a started
24-hour period counts in full.

When several requirements apply to the same stay, the first one in this order
decides the charge: PARK-1, PARK-8, PARK-2, PARK-7, PARK-5, PARK-6, then PARK-3
together with PARK-4.

## Charges

- PARK-1~1: WHEN the exit time precedes the entry time THE SYSTEM SHALL raise
  ValueError.
- PARK-2~1: WHEN a stay lasts 15 minutes or less THE SYSTEM SHALL charge 0.00.
- PARK-3~1: WHEN a stay of at most 24 hours lasts more than 15 minutes THE SYSTEM
  SHALL charge 2.50 per started hour, counted from the entry time.
- PARK-4~1: WHEN a stay of at most 24 hours lasts more than 15 minutes THE SYSTEM
  SHALL cap the charge at 18.00.
- PARK-5~1: WHEN a stay lasts more than 24 hours THE SYSTEM SHALL charge 15.00 per
  started 24-hour period, counted from the entry time.
- PARK-6~1: WHEN the entry time and the exit time both fall between 22:00 and
  06:00 of the same night and the stay lasts more than 2 hours THE SYSTEM SHALL
  charge 6.00.
- PARK-7~1: WHEN the ticket is validated and the stay lasts 3 hours or less THE
  SYSTEM SHALL charge 0.00, and SHALL charge a validated stay of more than 3 hours
  as if the ticket were not validated.
- PARK-8~1: WHEN the ticket is lost THE SYSTEM SHALL charge 40.00, whatever the
  entry time, the exit time and the validation.

## Receipt

- PARK-9~1: WHEN the amount is above 0.00 THE SYSTEM SHALL print the receipt as
  "EUR " followed by the amount with exactly two decimals.
- PARK-10~1: WHEN the amount is 0.00 THE SYSTEM SHALL print the receipt as "FREE".
