Below are the specification of a small Python library and its complete source code.

Write three mutants of the source: modified versions that a developer could plausibly produce by mistake, each violating this target requirement:

{target_ref}: {target_statement}

Slots:
- S1: violates the target requirement directly, for typical inputs it covers.
- S2: violates it only at a boundary or edge input of the requirement.
- S3: violates it only for a strict subset of the inputs it covers.

Every mutant must:
- keep every other requirement of the specification satisfied;
- use a mechanism different from the other two mutants;
- change one or two functions, each given as its complete new definition in `new_source`, replacing the function or method named by `qualname` (for a method, `ClassName.method`) in `file` (a path such as `src/parkfee/tariff.py`);
- change at most 10 lines in total;
- carry no comment or name that reveals the change.

`mechanism_summary` describes the change in one sentence.

Request id: {nonce}

=== Specification ===
{spec}

=== Source ===
{source}
