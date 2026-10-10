Python layout:
- Put the tests of each criterion in one file under `tests/spec_h/`, named `test_spec_h_c<n>_<name>.py`, where `<n>` is the criterion number. In a writing with a round prefix, insert it before `c<n>`: `test_spec_h_r2_c<n>_<name>.py` for round 2. There is no `conftest.py`.
- Use pytest. Write each test as a module-level function named `test_c<n>_<what>`, for example `test_c3_rejects_empty_input`.
- A property test uses Hypothesis: `@given(...)` with `@settings(derandomize=True, max_examples=100, deadline=None)`. An example test has no `@given`.
- Import only the standard library, pytest, Hypothesis and the modules named in the Interface.
