# Bicycle rental prices

The rental desk prices a bicycle rental by its duration in whole hours. Prices
are whole euros.

## Rental rates

- **BIKE-1~1**: WHEN a rental lasts from 1 to 10 whole hours THE SYSTEM SHALL charge 10.
  - Example: rental_price(hours=5) => 10
- **BIKE-2~1**: WHEN a rental lasts from 11 to 30 whole hours THE SYSTEM SHALL charge 18.
  - Example: rental_price(hours=20) => 18
- **BIKE-3~1**: WHEN a rental lasts from 31 to 80 whole hours THE SYSTEM SHALL charge 30.
  - Example: rental_price(hours=50) => 30
- **BIKE-4~1**: WHEN a rental lasts 81 or more whole hours THE SYSTEM SHALL charge 45.
  - Example: rental_price(hours=100) => 45
