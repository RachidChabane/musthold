package oracle

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

// ErrJUnit is returned for a report that is not a JUnit XML document.
var ErrJUnit = errors.New("the JUnit report is not readable")

// Case is one test case of a JUnit report.
type Case struct {
	ID     string
	Failed bool
}

type junitCase struct {
	Classname string     `xml:"classname,attr"`
	Name      string     `xml:"name,attr"`
	Failures  []struct{} `xml:"failure"`
	Errors    []struct{} `xml:"error"`
}

const testcaseElement = "testcase"

// caseID names a test as its class path and its name.
func caseID(classname, name string) string {
	if classname == "" {
		return name
	}
	return classname + nodeSeparator + name
}

// ParseJUnit reads every test case of a JUnit report. A case fails when it has a
// failure or an error child; a skipped case does not fail. A report without a
// test case is refused.
func ParseJUnit(data []byte) ([]Case, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var cases []Case
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrJUnit, err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != testcaseElement {
			continue
		}
		var raw junitCase
		if err := decoder.DecodeElement(&raw, &start); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrJUnit, err)
		}
		cases = append(cases, Case{ID: caseID(raw.Classname, raw.Name), Failed: len(raw.Failures)+len(raw.Errors) > 0})
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("%w: no test case", ErrJUnit)
	}
	return cases, nil
}
