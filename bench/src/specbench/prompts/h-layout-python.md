Python layout:
- Put each file under `tests/spec_h/`, named `test_spec_h_<name>.py` (the prefix keeps the file name unique in the repository).
- Use pytest. Name each test function `test_c<n>_<what>`, where `<n>` is the criterion number, for example `test_c3_rejects_empty_input`.
- Use Hypothesis for property tests with `@settings(derandomize=True, max_examples=100, deadline=None)` on every property test.
- Import only the standard library, pytest, Hypothesis and the modules named in the Interface.
