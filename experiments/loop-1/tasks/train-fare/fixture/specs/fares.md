# Train fares

The railway prices a single trip from the number of distance zones it crosses.
Fares are whole euros.

## Fares

- **FARE-1~1**: WHEN a trip crosses one or two zones THE SYSTEM SHALL charge 3.
  - Example: train_fare(zones=1) => 3
- **FARE-2~1**: WHEN a trip crosses three or four zones THE SYSTEM SHALL charge 5.
  - Example: train_fare(zones=3) => 5
- **FARE-3~1**: WHEN a trip crosses five or more zones THE SYSTEM SHALL charge 8.
  - Example: train_fare(zones=5) => 8
- **FARE-4~1**: WHEN a trip crosses zero or fewer zones THE SYSTEM SHALL charge 0, including negative zone counts.
  - Example: train_fare(zones=0) => 0
