# Deviations

- quota-pause | endpoints: none | call budget reached
- provenance | endpoints: none | CLI claude changed from 2.1.292 to 2.1.293
- quota-pause | endpoints: none | call budget reached
- quota-pause | endpoints: none | call budget reached
- quota-pause | endpoints: none | run-core stopped at a quota or rate limit (probe__J-H__tests_test_tariff.py__test_two_hour_stay_costs_two_started_hours__r2: ['claude exited 1']); a rerun of run-core resumes it
- quota-pause | endpoints: none | call budget reached
- protocol | endpoints: none | From 2026-10-09 the model-calling verbs run with --run-window always: the 21:00-07:30 default was set because the owner's Claude subscription quota was shared with the owner's daytime work, and the run now uses a separate account. PREREG 4.4 makes the window configurable; the call budget keeps its default of 250 new calls per local day and CLI, and no endpoint's inputs change.
- quota-pause | endpoints: none | call budget reached
- protocol | endpoints: none | From 2026-10-09 the model-calling verbs run with --call-budget 5000 per local day and CLI instead of the default 250: the default bounded the owner's Claude subscription quota, shared with the owner's own work, and the run now uses a separate account. A real quota or rate limit still stops a verb cleanly with a quota-pause entry and a rerun resumes it; no endpoint's inputs change.
- provenance | endpoints: none | CLI claude changed from 2.1.293 to 2.1.296
