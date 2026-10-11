package node

const (
	describeFunction = "describe"
	testFunction     = "test"

	jestDisplayName  = "Jest"
	jestExpectation  = "`expect` is a global: import nothing from jest."
	vitestDisplay    = "Vitest"
	vitestExpects    = "Import `expect` from 'vitest' in \"imports\" and nothing else from it."
	vitestSuiteAlias = "specproofDescribe"
	vitestTestAlias  = "specproofTest"
	vitestPreamble   = "import { describe as " + vitestSuiteAlias + ", test as " + vitestTestAlias + " } from 'vitest';"
)

func (jest) displayName() string { return jestDisplayName }

func (jest) expectation() string { return jestExpectation }

// preamble is empty: Jest provides describe and test as globals.
func (jest) preamble() string { return "" }

func (jest) registrars() (suite, test string) { return describeFunction, testFunction }

// testIdentity: jest-junit names a test and its classname by the titles of its
// blocks and its own, joined by a space.
func (jest) testIdentity(_, suite, test string) (classname, name string) {
	title := suite + jestTitleSeparator + test
	return title, title
}

func (vitest) displayName() string { return vitestDisplay }

func (vitest) expectation() string { return vitestExpects }

// preamble imports describe and test under names no witness can shadow.
func (vitest) preamble() string { return vitestPreamble }

func (vitest) registrars() (suite, test string) { return vitestSuiteAlias, vitestTestAlias }

// testIdentity: Vitest's JUnit reporter names a test by the titles of its blocks
// and its own joined by " > ", and its classname is the test file.
func (vitest) testIdentity(file, suite, test string) (classname, name string) {
	return file, suite + vitestTitleSeparator + test
}
