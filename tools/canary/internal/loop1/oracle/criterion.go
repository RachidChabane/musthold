package oracle

import (
	"path"
	"strings"
)

const (
	testFilePrefix   = "test_"
	testFileSuffix   = ".py"
	idSeparator      = "-"
	underscoreInFile = "_"
	nodeSeparator    = "::"
)

// CriterionOfFile maps an oracle test file to its criterion. The file is named
// test_<criterion>.py, the criterion written in lower case with underscores for
// hyphens: test_box_4.py holds the tests of BOX-4. The second result is false for
// a name that does not follow the convention.
func CriterionOfFile(file string) (string, bool) {
	base := path.Base(strings.ReplaceAll(file, `\`, "/"))
	if !strings.HasPrefix(base, testFilePrefix) || !strings.HasSuffix(base, testFileSuffix) {
		return "", false
	}
	stem := strings.TrimSuffix(strings.TrimPrefix(base, testFilePrefix), testFileSuffix)
	if stem == "" {
		return "", false
	}
	return strings.ToUpper(strings.ReplaceAll(stem, underscoreInFile, idSeparator)), true
}

// criterionOfTest maps a test id, a JUnit class path such as
// oracle.test_box_4.TestBox::test_free, or a file path used as a stand-in id, to
// its criterion; it is empty when no segment names a test file.
func criterionOfTest(id string) string {
	location := strings.SplitN(id, nodeSeparator, 2)[0]
	for _, segment := range strings.FieldsFunc(location, func(r rune) bool { return r == '.' || r == '/' }) {
		if criterion, ok := CriterionOfFile(segment + testFileSuffix); ok {
			return criterion
		}
	}
	return ""
}
