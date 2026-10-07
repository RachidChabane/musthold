Go layout:
- Put each file in the directory of a package named in the Interface, named `h_<name>_test.go`, and declare the package as `<package>_test` so that the file reaches the code only through its exported names.
- Use the standard `testing` package. Name each test function `TestH<n>_<What>`, where `<n>` is the criterion number, for example `TestH3_RejectsEmptyInput`.
- Import only the standard library and the packages named in the Interface. For property tests use `testing/quick` with a fixed `rand.New(rand.NewSource(<seed>))`, or table-driven cases that cover generated values from a fixed seed.
