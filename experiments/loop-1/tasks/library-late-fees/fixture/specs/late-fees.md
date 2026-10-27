# Library late fees

The library charges a fee for a loan according to its number of overdue days.
Fees are whole currency units.

## Fees

- **LATE-1~1**: WHEN a loan is not overdue THE SYSTEM SHALL charge 0.
  - Example: late_fee(days_overdue=0) => 0
- **LATE-2~1**: WHEN a loan is 1 to 5 days overdue THE SYSTEM SHALL charge 1.
  - Example: late_fee(days_overdue=3) => 1
- **LATE-3~1**: WHEN a loan is 6 to 10 days overdue THE SYSTEM SHALL charge 3.
  - Example: late_fee(days_overdue=7) => 3
- **LATE-4~1**: WHEN a loan is 11 to 20 days overdue THE SYSTEM SHALL charge 6.
  - Example: late_fee(days_overdue=15) => 6
- **LATE-5~1**: WHEN a loan is 21 or more days overdue THE SYSTEM SHALL charge 10, including at the 21-day boundary.
  - Example: late_fee(days_overdue=25) => 10
