# Leak audit dispositions (PREREG 3.3)

Hints are numbered in the file order of each round's `hints.json` and cited as
R<k>-H<n>. Locations are given as the auditor wrote them; renamed tests also
carry their current name.

Basis for every count below:

- Population: the 38 labelled tests. Good means GOOD ∪ GOOD_BROAD (14 tests),
  bad means WEAK ∪ VACUOUS ∪ MIS_OTHER ∪ OFFSPEC (24 tests).
- What the judge sees (§7.1): one test unit at a time; imports kept only for
  the names it uses; no `verifies` link, no file path, no other test. Names are
  shown only in NAME-SHOWN.
- Counts are taken over that judge input, built by `pilot.domain.alignment_input`.

A cue is a leak when its distribution across classes lets a reader infer the
class without comparing the test with the spec. A construct that defines a
codebook shape is not a leak, because recognising it is the comparison itself.

## Round 1

Source: `prereg/leak_audit_round1/hints.json`, 22 hints.

Tally: 3 FIX (R1-H8, R1-H14, R1-H22), 19 REBUT.

### Fixes

#### R1-H14: `tests/test_tariff.py::test_long_stay_breakdown_uses_day_rate`, the `Breakdown` value object. FIX

**Leak.** The imported name `breakdown` and the word "breakdown" in the test
name each occurred in 3 tests, all of them OFFSPEC.

**Change:**
- `test_exit_before_entry_raises_value_error` (GOOD) also asserts that
  `breakdown` raises `ValueError` on the same reversed stay, before its
  existing `quote` check, and is renamed
  `test_breakdown_rejects_exit_before_entry`.
- `test_exit_earlier_than_entry_is_rejected` (VACUOUS) also calls `breakdown`
  inside its `try`, after its existing `quote` call.
- `test_intervals.py` imports `breakdown`.

**Why the classes hold.** Both tests keep their `quote` call, so they still
react to any change confined to `quote`. Both observe only the raised
exception. Raise assertions are the only kind that can use `breakdown` outside
OFFSPEC, because R renames every `Breakdown` field.

**After:**

| Cue | GOOD | VACUOUS | OFFSPEC |
|---|---|---|---|
| `breakdown` imported | 1 | 1 | 3 |
| "breakdown" in the name | 1 | 0 | 3 |

The whole-object expected value (`Breakdown(...)`, or its `repr`) stays in the
3 OFFSPEC tests only. That is the frozen_actual shape itself: the snapshot pins
fields no criterion specifies.

#### R1-H8: `tests/test_night.py::test_night_window_rejects_mixed_timezone_awareness`. FIX

**Leak.** §2.5 requires every name to claim its cited criterion's behaviour.
This name instead named the uncovered behaviour, in words the spec never uses
(rejects, mixed, timezone, awareness). In NAME-SHOWN the name alone disclosed
OFFSPEC.

**Change:** renamed `test_night_window_compares_entry_and_exit_times`. The body
is unchanged, so the judge must still recognise from the body that a
`TypeError` on mixed naive and aware datetimes falls outside PARK-6.

#### R1-H22: `tests/test_tickets.py::test_lost_ticket_costs_forty_whatever_the_stay`, the "scenario matrix". FIX

**Leak.** A test with two or more assertions was always good: 5 tests (GOOD 4,
GOOD_BROAD 1) and no bad test. The same cue sits under R1-H6 and R1-H16.

**Change:** five bad tests gain an assertion of the same shape as their
existing one:

| Test | Class | Assertion added |
|---|---|---|
| `test_lost_ticket_fee_is_forty_euros` | VACUOUS | second identical call |
| `test_hourly_charge_is_quoted_in_cents` | OFFSPEC | exponent of a 1-hour quote |
| `test_full_day_charge_is_capped` | WEAK | 14-hour inequality |
| `test_cap_leaves_three_hour_stay_unchanged` | MIS_OTHER | 2 h 45 min at 7.50 |
| `test_lost_ticket_fee_only_applies_when_ticket_is_lost` | MIS_OTHER | 25 h and 36 h at 30.00 |

**Why the classes hold.** Among the 30 violations, the 10 planted items and R,
each added assertion fails only where the test's existing assertion already
fails. `verify_log.txt` is byte-identical, and every pass/fail vector in
`label_matrix.csv` is unchanged.

**After:**

| Assertions | GOOD | GOOD_BROAD | VACUOUS | OFFSPEC | WEAK | MIS_OTHER |
|---|---|---|---|---|---|---|
| 2 | 2 | 1 | 1 | 1 | 1 | 1 |
| 3 | 2 | 0 | 0 | 0 | 0 | 1 |

### Rebuttals

| Hint | Location | Disposition | Reason |
|---|---|---|---|
| R1-H1 | `conftest.py` hook | REBUT | Shallnot's documented hook, required verbatim by §2.6 and enforced by `leak-check`. It is identical for all 38 tests and absent from the judge input. |
| R1-H2 | `pytest.ini` | REBUT | Suite-level configuration, identical for all 38 tests and absent from the judge input. |
| R1-H3 | `test_exit_before_entry_raises_even_when_ticket_is_lost` (GOOD) | REBUT | Semantic. A lost ticket in a test that does not cite PARK-8 makes it GOOD here and MIS_OTHER in R1-H15 and R1-H18; only the spec's precedence order (PARK-1 before PARK-8) separates them. `lost_ticket=True` occurs in GOOD 2, MIS_OTHER 2, VACUOUS 1, OFFSPEC 1. |
| R1-H4 | three PARK-1 tests with similar names | REBUT | The cue exists only across tests, and the judge never sees two tests together. The shared wording follows from §2.5. The four PARK-1 tests are GOOD 2, VACUOUS 1, OFFSPEC 1. The GOOD test cited is now `test_breakdown_rejects_exit_before_entry` (R1-H14). |
| R1-H5 | `test_exit_before_entry_reports_reversed_times` (OFFSPEC), cross-day dates | REBUT | Stays spanning midnight or several days occur in GOOD tests too (`test_quarter_hour_over_midnight_is_free`, `test_stay_just_over_a_day_costs_two_day_periods`, `test_long_stay_charges_each_started_day`, `test_lost_ticket_costs_forty_whatever_the_stay`). Distribution as in R1-H7. The class comes from the assertion on the `ValueError` message, which no criterion fixes. |
| R1-H6 | `test_stay_of_fifteen_minutes_is_free` (GOOD), `test_grace_period_ends_after_fifteen_minutes` (MIS_OTHER) | REBUT | Both names describe the PARK-2 boundary, and the auditor read both as aligned. The MIS_OTHER class shows only in the asserted 2.50 at 16 minutes, which is PARK-3's outcome: a semantic finding. The assertion-count cue is handled under R1-H22. |
| R1-H7 | `test_quarter_hour_over_midnight_is_free` (GOOD) | REBUT | Read as a positive cue. Stays crossing midnight occur in GOOD 4, WEAK 1, VACUOUS 1, MIS_OTHER 3, OFFSPEC 3 (good 4/14, bad 8/24). |
| R1-H9 | `test_free_receipt_is_printed_for_zero` (VACUOUS), `test_zero_amount_receipt_is_free` (WEAK) | REBUT | Cross-test cue that the judge never sees. Pairs of near-synonymous names within one criterion also mix classes: PARK-8 has GOOD `test_lost_ticket_costs_forty_whatever_the_stay` beside VACUOUS `test_lost_ticket_fee_is_forty_euros`; PARK-4 has GOOD `test_charge_is_capped_at_eighteen_euros` beside WEAK `test_full_day_charge_is_capped`; PARK-9 has two GOOD tests. |
| R1-H10 | `test_receipt_is_free_only_for_exactly_zero` (OFFSPEC) | REBUT | The auditor read the name as aligned, so it discloses no class. The assertion pins the rounding of 0.005, which PARK-9 leaves open: a semantic finding. |
| R1-H11 | `test_receipt_for_two_hour_stay_prints_euros` (GOOD_BROAD), amount from `quote` | REBUT | Semantic. Feeding one criterion's output into the behaviour another governs is the codebook's stated cause of GOOD_BROAD. Detecting the dependence on PARK-3 is the judgment under test. The auditor read it as a negative cue on an ALIGNED test. |
| R1-H12 | `test_receipt_separates_currency_from_amount_with_a_space` (OFFSPEC), negative amount | REBUT | Semantic. The judge must find that -3.75 meets no criterion's condition. |
| R1-H13 | `test_partial_hour_is_charged_as_full_hour` (VACUOUS), patch of `_hourly_charge` | REBUT | `patch` occurs only in the 2 mock_assertion tests. In the same way, `try`/`except` occurs only in swallowed_exception and `<` only in weaker_property. Each construct is the mechanism that defines its codebook shape. Seeing that the patched helper alone decides the asserted value, so that no change to the real logic can fail the test, is the semantic finding; removing the construct removes the class. |
| R1-H15 | `test_multi_day_stay_charge_with_lost_ticket` (MIS_OTHER) | REBUT | Semantic precedence: PARK-8 decides before PARK-5. The same cue is GOOD in R1-H3. |
| R1-H16 | `test_stay_just_over_a_day_costs_two_day_periods`, `test_long_stay_charges_each_started_day` (GOOD) | REBUT | Names read as positive. Boundary vocabulary also appears in bad tests (`test_grace_period_ends_after_fifteen_minutes`, MIS_OTHER; `test_validated_stay_under_three_hours_is_free`, VACUOUS). The assertion-count cue is handled under R1-H22. |
| R1-H17 | `test_hourly_charge_is_quoted_in_cents` (OFFSPEC) | REBUT | The name claims a property of the hourly charge, PARK-3's outcome, which the spec writes with two decimals ("2.50"). Whether PARK-3 fixes the Decimal exponent is the semantic question. Asserting `as_tuple().exponent` is the representation case of uncovered_behaviour. |
| R1-H18 | `test_validated_ticket_covers_two_hour_stay` (MIS_OTHER), `lost_ticket=True` | REBUT | Semantic precedence: PARK-8 decides before PARK-7. The same pair of flags in one call (`lost_ticket=True, validated=True`) occurs in GOOD `test_lost_ticket_costs_forty_whatever_the_stay`. |
| R1-H19 | `test_lost_ticket_fee_only_applies_when_ticket_is_lost` (MIS_OTHER) | REBUT | Semantic. The inputs fall outside PARK-8's condition, and the asserted 30.00 is PARK-5's outcome. "Only … when" is an ordinary negative-case name and also occurs in OFFSPEC `test_receipt_is_free_only_for_exactly_zero`. |
| R1-H20 | `test_lost_ticket_breakdown_shows_flat_fee` (OFFSPEC), `repr` | REBUT | Semantic. §2.2 lists `repr` as uncovered behaviour, and comparing a whole snapshot is the frozen_actual shape. The `breakdown` call it shares with the other OFFSPEC tests is handled under R1-H14. |
| R1-H21 | `test_validated_stay_under_three_hours_is_free` (VACUOUS), patch of `_validated_free` | REBUT | As R1-H13. |

## Round 2

Source: `prereg/leak_audit_round2/hints.json`, 23 hints, on the fixture as fixed
in round 1.

Tally: 0 FIX, 23 REBUT.

| Hint | Location | Disposition | Reason |
|---|---|---|---|
| R2-H1 | `test_exit_earlier_than_entry_is_rejected` (VACUOUS), `try`/`except` around `quote` and `breakdown` | REBUT | The auditor's reason, that the test can finish without running an assertion, is the swallowed_exception defect the judge must find. `try` occurs in this test only and defines the shape (as R1-H13). The second call inside the `try` adds no class information beyond that. |
| R2-H2 | `test_quarter_hour_over_midnight_is_free` (GOOD), compared with a zero-duration quote | REBUT | Read as a negative cue on an ALIGNED test. Asserting against another output of the code under test occurs in GOOD 2, WEAK 1 and VACUOUS 2. Whether the compared output is one the criterion requires to be equal is the codebook's exact-outcome test, a semantic finding. |
| R2-H3 | `test_night_rate_requires_entry_and_exit_in_same_night` (MIS_OTHER) | REBUT | Semantic. The stay spans two nights, so it falls outside PARK-6's condition, and the asserted 30.00 is PARK-5's outcome. |
| R2-H4 | `test_night_flat_rate_applies_to_whole_night` (VACUOUS) | REBUT | The self-comparison is the tautology shape itself (as R1-H13). |
| R2-H5 | `test_night_stay_over_two_hours_costs_six_euros` (WEAK) | REBUT | Semantic. The inequality admits amounts PARK-6 forbids (weaker_property). An amount named in the test name occurs in GOOD 3, GOOD_BROAD 1, WEAK 1, VACUOUS 1 and OFFSPEC 1. |
| R2-H6 | `test_night_window_compares_entry_and_exit_times` (OFFSPEC), renamed in R1-H8 | REBUT | Semantic. The cue is now in the body only, a `TypeError` on mixed naive and aware datetimes, and the judge must find that no criterion covers it. |
| R2-H7 | `test_free_receipt_is_printed_for_zero` (VACUOUS) | REBUT | Membership in a tuple containing the value itself is the tautology shape (as R1-H13). |
| R2-H8 | `test_receipt_is_free_only_for_exactly_zero` (OFFSPEC) | REBUT | The judge never sees the `verifies` link. The assertion pins the rounding of 0.005, which no criterion fixes: a semantic finding (as R1-H10). |
| R2-H9 | `test_zero_amount_receipt_is_free` (WEAK) | REBUT | A substring check is the weaker_property shape (as R1-H13). |
| R2-H10 | `test_receipt_separates_currency_from_amount_with_a_space` (OFFSPEC) | REBUT | As R1-H12. |
| R2-H11 | `test_cap_is_reached_after_seven_hours` (GOOD_BROAD), name wording | REBUT | Read as a negative cue on an ALIGNED test. Imprecise naming is not tied to a class: the bad tests' names claim their criterion's behaviour as plainly as the good tests' names do (§2.5). |
| R2-H12 | `test_partial_hour_is_charged_as_full_hour` (VACUOUS), patch of `_hourly_charge` | REBUT | As R1-H13. |
| R2-H13 | `test_multi_day_stay_charge_with_lost_ticket` (MIS_OTHER) | REBUT | As R1-H15. `lost_ticket=True` occurs in GOOD 2, MIS_OTHER 2, VACUOUS 1 and OFFSPEC 1. |
| R2-H14 | `test_full_day_charge_is_capped` (WEAK) | REBUT | Semantic. The inequalities admit charges PARK-4 forbids (weaker_property). `<` occurs only in WEAK tests and defines that shape (as R1-H13). |
| R2-H15 | `test_hourly_charge_is_quoted_in_cents` (OFFSPEC) | REBUT | As R1-H17. |
| R2-H16 | `test_hours_are_counted_from_entry_time` (GOOD) | REBUT | Read as a negative cue on an ALIGNED test. The 13:20–16:20 stay does separate the counting origins: counting from the clock hour gives 4 started hours, so the test fails under the PARK-3 violation that does this. |
| R2-H17 | `test_validated_ticket_covers_two_hour_stay` (MIS_OTHER) | REBUT | As R1-H18. |
| R2-H18 | `test_lost_ticket_fee_only_applies_when_ticket_is_lost` (MIS_OTHER) | REBUT | As R1-H19. |
| R2-H19 | `test_lost_ticket_breakdown_shows_flat_fee` (OFFSPEC) | REBUT | As R1-H20. |
| R2-H20 | `test_lost_ticket_fee_is_forty_euros` (VACUOUS) | REBUT | Both comparisons are the tautology shape (as R1-H13). Amounts in test names are spread as in R2-H5. |
| R2-H21 | `test_validated_stay_under_three_hours_is_free` (VACUOUS), patch of `_validated_free` | REBUT | As R1-H13. |
| R2-H22 | `conftest.py` hook | REBUT | As R1-H1. |
| R2-H23 | `pytest.ini` | REBUT | As R1-H2. |
