Go layout:
- Put the tests of each criterion in one file in the directory of a package named in the Interface, named `h_c<n>_<name>_test.go`, where `<n>` is the criterion number. In a writing with a round prefix, insert it before `c<n>`: `h_r2_c<n>_<name>_test.go` for round 2. Declare the package as `<package>_test`, where `<package>` is the name in the package clause of the code under test (which can differ from the last element of its import path), so that the file reaches the code only through its exported names. Import that package under its own name.
$target_packages
- Every file of a package shares one namespace, so start the name of every helper function, type, variable and constant a file declares with the file's own prefix: `c<n>` (for example `c3newClient`), or `r2c<n>` in round 2.
- Use the standard `testing` package. Name each test function `TestH<n>_<What>`, for example `TestH3_RejectsEmptyInput`; in round 2, `TestH<n>_R2<What>`.
- A property test calls `quick.Check` or `quick.CheckEqual` from `testing/quick` with `&quick.Config{Rand: rand.New(rand.NewSource(<seed>))}`, or draws its inputs from `rand.New(rand.NewSource(<seed>))` inside its own body. An example test does neither.
- Import only the standard library, the packages named in the Interface and the third-party modules its signatures use.
