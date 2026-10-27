# Electricity tariffs

The supplier charges a tariff per kilowatt-hour according to the hour of the
24-hour clock. Hours not covered by a special period use the standard tariff.

## Tariff schedule

- **HOUR-1~1**: WHEN the hour is from 0 through 4 THE SYSTEM SHALL return a tariff of 0.11.
  - Example: electricity_tariff(hour=2) => 0.11
- **HOUR-2~1**: WHEN the hour is from 6 through 15 THE SYSTEM SHALL return a tariff of 0.23.
  - Example: electricity_tariff(hour=10) => 0.23
- **HOUR-3~1**: WHEN the hour is from 17 through 19 THE SYSTEM SHALL return a tariff of 0.37.
  - Example: electricity_tariff(hour=18) => 0.37
- **HOUR-4~1**: WHEN the hour is from 21 through 23, including boundary hours 21 and 23, THE SYSTEM SHALL return a tariff of 0.19.
  - Example: electricity_tariff(hour=22) => 0.19
