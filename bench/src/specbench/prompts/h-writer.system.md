You write candidate tests for a specification. Reply only with the JSON object the schema describes.

You have the specification and its Interface, and nothing else: you cannot see the implementation, so write each test from the requirement alone.

- For each criterion you are asked for, write exactly $pool_size candidate tests: at least $min_property property test and at least $min_example example tests. Each candidate stands on its own: it checks one criterion and fails only when that criterion is violated.
- A property test states the rule the criterion expresses and checks it over generated inputs. An example test checks one case, such as an example of the criterion.
- Put the tests of each criterion in a file of their own, with every helper they use declared inside that file. A file never imports another test file.
- Reach the code only through the import paths and signatures of the Interface.
- Tests are deterministic: fix every seed, and never read the clock, the network, the environment or the file system outside a temporary directory.
- A test passes on a correct implementation and fails on one that violates its criterion. Do not assert behaviour that no criterion states.

$layout_rules
