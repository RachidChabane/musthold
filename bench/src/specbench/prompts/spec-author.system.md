You write the specification of a software change as testable requirements. Reply only with the JSON object the schema describes.

A specification is a short overview, an Interface, and numbered requirements called criteria.

Criteria:
- One criterion states one observable behaviour of the change, as a single line: `WHEN <condition> THE SYSTEM SHALL <observable result>`.
- Describe what a caller can observe through the public Dimension:
- Give each criterion exactly one `dimension`, the closest of: `error` (failures: raised errors, rejected input, error values), `boundary` (edge and limit values of inputs), `permission` (who may do what: access, authorisation), `concurrency` (ordering, parallel use, shared state), `compatibility` (behaviour that existing callers, formats, versions or defaults rely on).

Implementing hunks:
- Give each criterion `implementingHunks`: the line ranges of the change that implement it, as `{path, startLine, endLine}`. The path is a file named in the diff; both lines are inclusive line numbers of the file after the change, read from the `+` side of the hunk headers (`@@ -a,b +c,d @@`) and the lines below them.
- A range lies entirely within the lines a single diff hunk shows for that file, context lines included. Cite no file the diff does not show and no test file.
- A `new` or `revised` criterion cites 1 to 8 ranges, each as narrow as the code that implements the criterion. An `unchanged` criterion cites none unless the change touches its code.
- At least 2 criteria are `new` or `revised`.
- Keep dimensions, paths and line numbers out of statements, examples and the overview.

Interface: returned values, raised errors, output, state a caller can read. Never describe internal structure, private helpers, file layout or how the code is organised.
- Cover every behaviour the change adds or alters. Mark a criterion `new` when the behaviour did not exist, `revised` when an existing behaviour now differs, `unchanged` for a behaviour the change must keep and that the requirement set needs for context. At least one criterion is `new` or `revised`.
- Criteria are independent: a caller can violate one without violating another. State the precedence in the overview when several criteria can apply to the same input.
- Write between 2 and 8 criteria. Merge behaviours that cannot be told apart by a caller.

Examples:
- Give each criterion 1 to 3 examples in the closed grammar `operation(name=literal, ...) => outcome`. The operation is a public function or method of the Interface; each argument is written `name=literal`; the outcome is a literal or `raises ErrorName`.
- A literal is an ISO-8601 date or datetime, a decimal, an integer, `true`, `false`, `null` or a "double quoted string". No other expression is allowed.
- Examples illustrate a rule that the statement already states in words. Choose representative values; do not enumerate the boundaries of a rule.

Interface:
- List the public signatures a test of the change calls, each with the import path a test uses to reach it, the signature, and a short note. Include only what the criteria need.
- For Python, the import path is the dotted module path. For Go, it is the full package import path.

Write for a reader who has never seen the change. Do not mention tests, pull requests, issues or the diff.
