package pytest

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/discpower/internal/platform/witnesskit"
)

var _ ports.Witnessing = (*Strategy)(nil)

const (
	witnessLanguage = "Python, pytest"
	// witnessConventions tells a writer how a pytest witness is written. It reads
	// no test of the project.
	witnessConventions = "Each witness is a pytest test function at the top level of a test module of its own: `def <name>():`, " +
		"or with parameters that name fixtures of the project's conftest files. Assert with plain `assert` statements. " +
		"Import the code under test by the module paths the interface names, and a helper of a test module by its module path. " +
		"A property draws its inputs from `random.Random(<integer constant>)`, never from the global random state or the clock."
	witnessFilePrefix   = "test_specproof_"
	witnessDefaultDir   = "tests"
	witnessTestPrefix   = "test_"
	defKeyword          = "def"
	classKeyword        = "class"
	headerTerminator    = ':'
	privatePrefix       = "_"
	testFunctionPrefix  = "test"
	testClassPrefix     = "Test"
	sourceRootPrefix    = "src/"
	packageInitModule   = "__init__"
	qualifiedSeparator  = "."
	witnessFileDigestLn = 12
	sectionGap          = "\n\n\n"
)

// Witnesses is the witness host of pytest projects.
func (s *Strategy) Witnesses() ports.WitnessHost { return witnessHost{strategy: s} }

type witnessHost struct {
	strategy *Strategy
}

// Brief lists the headers of the public definitions of the source files the spec
// mentions, by module, and the headers of the helpers and fixtures of every test
// file of the project.
func (h witnessHost) Brief(project fs.FS, sources []mutation.SourceFile, specText string) (ports.WitnessBrief, error) {
	var entries []witnesskit.Entry
	for _, source := range sources {
		if !strings.HasSuffix(source.Path, pythonSuffix) {
			continue
		}
		found, err := h.publicEntries(source)
		if err != nil {
			return ports.WitnessBrief{}, err
		}
		entries = append(entries, found...)
	}
	helpers, err := h.helpers(project)
	if err != nil {
		return ports.WitnessBrief{}, err
	}
	return ports.WitnessBrief{
		Language: witnessLanguage, Conventions: witnessConventions,
		Interface: witnesskit.Choose(entries, witnesskit.Mentions(specText)), Helpers: helpers,
	}, nil
}

// publicEntries lists the header of every public function, class and method of
// a source, with the fields a dataclass declares.
func (h witnessHost) publicEntries(source mutation.SourceFile) ([]witnesskit.Entry, error) {
	definitions, err := h.strategy.deps.Locator.Locate(source.Path, source.Content)
	if err != nil {
		return nil, fmt.Errorf("locate the definitions of %q: %w", source.Path, err)
	}
	module := modulePath(source.Path)
	var entries []witnesskit.Entry
	for _, definition := range definitions {
		names := strings.Split(definition.Qualname, qualifiedSeparator)
		if !public(names) {
			continue
		}
		text, err := definition.Span.Slice(source.Content)
		if err != nil {
			return nil, err
		}
		entries = append(entries, witnesskit.Entry{Names: names, Signature: module + ": " + h.header(string(text))})
	}
	return entries, nil
}

// header renders a definition without its body: decorators and the line that
// opens it, and for a class the field annotations of its body.
func (h witnessHost) header(text string) string {
	head := witnesskit.Header(text, headerTerminator)
	if !strings.HasPrefix(strings.TrimSpace(lastLine(head)), classKeyword+" ") {
		return head
	}
	var fields []string
	for _, line := range strings.Split(text, "\n")[1:] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "     ") && isFieldAnnotation(trimmed) {
			fields = append(fields, "    "+trimmed)
		}
	}
	if len(fields) == 0 {
		return head
	}
	return head + ":\n" + strings.Join(fields, "\n")
}

func lastLine(text string) string {
	lines := strings.Split(text, "\n")
	return lines[len(lines)-1]
}

// isFieldAnnotation reports whether a class body line declares an annotated
// field: `name: type` with an optional default.
func isFieldAnnotation(line string) bool {
	name, rest, found := strings.Cut(line, ":")
	return found && identifierPattern.MatchString(name) && strings.TrimSpace(rest) != ""
}

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func public(names []string) bool {
	for _, name := range names {
		if strings.HasPrefix(name, privatePrefix) {
			return false
		}
	}
	return true
}

// helpers lists the headers of the top-level definitions of every test file that
// are not tests: helper functions and classes, and fixtures with their
// decorator.
func (h witnessHost) helpers(project fs.FS) ([]string, error) {
	files, err := witnesskit.TestFiles(project, h.strategy.Layout(), nil)
	if err != nil {
		return nil, err
	}
	var helpers []string
	for _, file := range files {
		content, err := fs.ReadFile(project, file)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", file, err)
		}
		definitions, err := h.strategy.deps.Locator.Locate(file, content)
		if err != nil {
			continue
		}
		for _, definition := range definitions {
			if strings.Contains(definition.Qualname, qualifiedSeparator) || isTestName(definition.Qualname) {
				continue
			}
			text, err := definition.Span.Slice(content)
			if err != nil {
				return nil, err
			}
			helpers = append(helpers, modulePath(file)+": "+witnesskit.Header(string(text), headerTerminator))
		}
	}
	return helpers, nil
}

func isTestName(name string) bool {
	return strings.HasPrefix(name, testFunctionPrefix) || strings.HasPrefix(name, testClassPrefix)
}

// modulePath is the dotted module path of a Python file, its `src/` root left
// out.
func modulePath(file string) string {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(file, sourceRootPrefix), pythonSuffix)
	trimmed = strings.TrimSuffix(trimmed, pathSeparator+packageInitModule)
	return strings.ReplaceAll(trimmed, pathSeparator, moduleSeparator)
}

// Layout writes a set as one test module in the directory of the project's first
// test module, each witness renamed after its short id.
func (h witnessHost) Layout(project fs.FS, set witness.Set) (ports.WitnessFile, error) {
	directory, err := h.witnessDirectory(project)
	if err != nil {
		return ports.WitnessFile{}, err
	}
	file := path.Join(directory, witnessFilePrefix+set.Digest()[:witnessFileDigestLn]+pythonSuffix)
	classname := strings.ReplaceAll(strings.TrimSuffix(file, pythonSuffix), pathSeparator, moduleSeparator)
	var content strings.Builder
	content.WriteString(strings.TrimSpace(set.Imports) + "\n")
	tests := map[string]ecosystem.BoundTest{}
	for _, w := range set.Witnesses {
		renamed := witnesskit.RenamedName(witnessTestPrefix, w.ShortID())
		code, declared := witnesskit.Rename(w.Code, witnesskit.Declaration(defKeyword, w.Name), renamed)
		content.WriteString(sectionGap)
		if !declared {
			continue
		}
		line := witnesskit.LineOf(content.String(), content.Len())
		content.WriteString(strings.TrimRight(code, "\n") + "\n")
		tests[w.ID] = ecosystem.NewBoundTest(classname, renamed, file, line)
	}
	return ports.WitnessFile{Path: file, Content: []byte(content.String()), Tests: tests}, nil
}

// witnessDirectory is the directory of the project's first test module, or
// `tests` when the project has none.
func (h witnessHost) witnessDirectory(project fs.FS) (string, error) {
	files, err := witnesskit.TestFiles(project, h.strategy.Layout(), nil)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if strings.HasPrefix(path.Base(file), witnessTestPrefix) {
			return path.Dir(file), nil
		}
	}
	return witnessDefaultDir, nil
}
