package node

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/discpower/internal/platform/witnesskit"
)

var _ ports.Witnessing = (*Strategy)(nil)

const (
	typeScriptName = "TypeScript"
	javaScriptName = "JavaScript"

	witnessFilePrefix   = "musthold_"
	witnessFileMarker   = ".test"
	witnessDefaultDir   = "test"
	witnessDigestLength = 12
	// witnessSuite is the title of the describe block that registers the
	// witnesses of a file; it keeps their full titles independent of how the
	// reporters name a test outside any block.
	witnessSuite = "musthold_witnesses"

	functionKeyword = "function"
	asyncKeyword    = "async"

	typeScriptExtension = ".ts"
	javaScriptExtension = ".js"
	moduleTypeESM       = "module"
	currentDirectory    = "./"
	parentDirectory     = ".."
	pathSeparator       = "/"
	voidReturnType      = ": void"
	exampleSource       = "src/module"
)

var (
	typeScriptExtensions = []string{".ts", ".tsx", ".mts", ".cts"}
	scriptExtensions     = append([]string{".js", ".jsx", ".mjs", ".cjs"}, typeScriptExtensions...)
	testFileMarkers      = []string{".test.", ".spec."}
)

const declarationFileEnd = ".d.ts"

const witnessConventionsFormat = "Each witness is a plain function declared at the top level of the witness file: " +
	"`function <name>()%[1]s {` or `async function <name>()%[1]s {` at the start of a line, with no `export` and no parameters. " +
	"It throws when the behaviour is wrong: assert with `expect(...)`. %[2]s " +
	"Declare nothing at the top level besides the function and register nothing: the file registers each function as a test itself, " +
	"and the names `describe`, `test` and `it` are taken. " +
	"In \"imports\", write one statement per line, resolved from directory %[3]s of the witness file (for example `%[4]s`), " +
	"and use every one of them: an unused import is an error. " +
	"A property draws its inputs from a pseudo-random generator written inside the function (such as mulberry32) seeded with a fixed integer constant, " +
	"never from Math.random or the clock."

// witnessFramework is what differs between the Node runners where witnesses are
// laid out.
type witnessFramework interface {
	// displayName names the runner in the brief.
	displayName() string
	// expectation tells where `expect` comes from.
	expectation() string
	// preamble is the code the witness file starts with, empty when the runner
	// provides its test functions as globals.
	preamble() string
	// registrars are the names the file calls to open a suite and to register a
	// test.
	registrars() (suite, test string)
	// testIdentity is the classname and the name the runner's JUnit reporter
	// gives a test registered in the witness suite.
	testIdentity(file, suite, test string) (classname, name string)
}

// Witnesses is the witness host of Jest and Vitest projects.
func (s *Strategy) Witnesses() ports.WitnessHost { return witnessHost{strategy: s} }

type witnessHost struct {
	strategy *Strategy
}

// Brief lists the exported declarations of the sources the spec mentions,
// without bodies, each under the specifier the witness file imports it by, and
// the exported declarations of every test file.
func (h witnessHost) Brief(project fs.FS, sources []mutation.SourceFile, specText string) (ports.WitnessBrief, error) {
	directory, err := h.witnessDirectory(project)
	if err != nil {
		return ports.WitnessBrief{}, err
	}
	var entries []witnesskit.Entry
	example := ""
	for _, source := range sources {
		if !isScript(source.Path) {
			continue
		}
		found, err := h.entries(source.Path, source.Content, directory, false)
		if err != nil {
			return ports.WitnessBrief{}, err
		}
		if example == "" {
			example = importSpecifier(directory, source.Path)
		}
		entries = append(entries, found...)
	}
	if example == "" {
		example = importSpecifier(directory, exampleSource)
	}
	helpers, err := h.helpers(project, directory)
	if err != nil {
		return ports.WitnessBrief{}, err
	}
	typed := h.typeScript(project)
	return ports.WitnessBrief{
		Language:    h.language(typed),
		Conventions: h.conventions(project, typed, directory, example),
		Interface:   witnesskit.Choose(entries, witnesskit.Mentions(specText)),
		Helpers:     helpers,
	}, nil
}

func (h witnessHost) language(typed bool) string {
	name := javaScriptName
	if typed {
		name = typeScriptName
	}
	return name + ", " + h.strategy.framework.displayName()
}

func (h witnessHost) conventions(project fs.FS, typed bool, directory, example string) string {
	returnType := ""
	if typed {
		returnType = voidReturnType
	}
	esm := typed || usesESModules(project)
	return fmt.Sprintf(witnessConventionsFormat, returnType, h.strategy.framework.expectation(), directory, importStatement(esm, example))
}

func importStatement(esm bool, specifier string) string {
	if esm {
		return "import { name } from '" + specifier + "';"
	}
	return "const { name } = require('" + specifier + "');"
}

// helpers lists the exported declarations of every test file.
func (h witnessHost) helpers(project fs.FS, directory string) ([]string, error) {
	files, err := witnesskit.TestFiles(project, h.strategy.Layout(), nil)
	if err != nil {
		return nil, err
	}
	var helpers []string
	for _, file := range files {
		if !isScript(file) {
			continue
		}
		content, err := fs.ReadFile(project, file)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", file, err)
		}
		found, err := h.entries(file, content, directory, true)
		if err != nil {
			continue
		}
		for _, entry := range found {
			helpers = append(helpers, entry.Signature)
		}
	}
	return helpers, nil
}

// laidWitness is a witness whose function has been renamed.
type laidWitness struct {
	id      string
	renamed string
}

// Layout writes a set as one test file next to the project's first test file:
// the imports, each witness renamed after its short id, and one suite that
// registers them.
func (h witnessHost) Layout(project fs.FS, set witness.Set) (ports.WitnessFile, error) {
	directory, err := h.witnessDirectory(project)
	if err != nil {
		return ports.WitnessFile{}, err
	}
	file := path.Join(directory, witnessFilePrefix+set.Digest()[:witnessDigestLength]+witnessFileMarker+h.scriptExtension(project))
	framework := h.strategy.framework
	var content strings.Builder
	if preamble := framework.preamble(); preamble != "" {
		content.WriteString(preamble + "\n")
	}
	if imports := strings.TrimSpace(set.Imports); imports != "" {
		content.WriteString(imports + "\n")
	}
	var laid []laidWitness
	for _, w := range set.Witnesses {
		renamed := witnesskit.RenamedName("", w.ShortID())
		code, declared := renameFunction(w.Code, w.Name, renamed)
		if !declared {
			continue
		}
		content.WriteString("\n" + strings.TrimRight(code, "\n") + "\n")
		laid = append(laid, laidWitness{id: w.ID, renamed: renamed})
	}
	suite, register := framework.registrars()
	fmt.Fprintf(&content, "\n%s(%q, () => {\n", suite, witnessSuite)
	tests := map[string]ecosystem.BoundTest{}
	for _, item := range laid {
		line := witnesskit.LineOf(content.String(), content.Len())
		fmt.Fprintf(&content, "  %s(%q, %s);\n", register, item.renamed, item.renamed)
		classname, name := framework.testIdentity(file, witnessSuite, item.renamed)
		tests[item.id] = ecosystem.NewBoundTest(classname, name, file, line)
	}
	content.WriteString("});\n")
	return ports.WitnessFile{Path: file, Content: []byte(content.String()), Tests: tests}, nil
}

// renameFunction renames the first top-level declaration of a function, plain
// or async.
func renameFunction(code, name, renamed string) (string, bool) {
	pattern := regexp.MustCompile(`(?m)^[ \t]*(?:` + asyncKeyword + `[ \t]+)?` + functionKeyword + `[ \t]+(` + regexp.QuoteMeta(name) + `)[ \t]*\(`)
	return witnesskit.Rename(code, pattern, renamed)
}

// witnessDirectory is the directory of the project's first test file, or `test`
// when the project has none.
func (h witnessHost) witnessDirectory(project fs.FS) (string, error) {
	first, err := h.firstTestFile(project)
	if err != nil {
		return "", err
	}
	if first == "" {
		return witnessDefaultDir, nil
	}
	return path.Dir(first), nil
}

// firstTestFile is the first file the project names a test, not its setup or
// configuration.
func (h witnessHost) firstTestFile(project fs.FS) (string, error) {
	files, err := witnesskit.TestFiles(project, h.strategy.Layout(), nil)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		base := path.Base(file)
		if isScript(file) && !strings.HasPrefix(base, witnessFilePrefix) && slices.ContainsFunc(testFileMarkers, func(marker string) bool { return strings.Contains(base, marker) }) {
			return file, nil
		}
	}
	return "", nil
}

// scriptExtension is the extension of the witness file.
func (h witnessHost) scriptExtension(project fs.FS) string {
	if h.typeScript(project) {
		return typeScriptExtension
	}
	return javaScriptExtension
}

// typeScript reports whether the project's first test file is TypeScript, or,
// when it has none, whether the project has a tsconfig.
func (h witnessHost) typeScript(project fs.FS) bool {
	if first, err := h.firstTestFile(project); err == nil && first != "" {
		return slices.Contains(typeScriptExtensions, path.Ext(first))
	}
	_, err := fs.Stat(project, tsconfigFile)
	return err == nil
}

// usesESModules reports whether package.json declares the project's JavaScript
// as ES modules.
func usesESModules(project fs.FS) bool {
	content, err := fs.ReadFile(project, packageFile)
	if err != nil {
		return false
	}
	var manifest struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(content, &manifest) == nil && manifest.Type == moduleTypeESM
}

// importSpecifier is the path the witness file in a directory imports a source
// by: relative, without the extension.
func importSpecifier(directory, source string) string {
	from := strings.Split(path.Clean(directory), pathSeparator)
	to := strings.Split(strings.TrimSuffix(source, path.Ext(source)), pathSeparator)
	common := 0
	for common < len(from) && common < len(to)-1 && from[common] == to[common] {
		common++
	}
	var steps []string
	for range from[common:] {
		steps = append(steps, parentDirectory)
	}
	specifier := strings.Join(append(steps, to[common:]...), pathSeparator)
	if !strings.HasPrefix(specifier, parentDirectory) {
		specifier = currentDirectory + specifier
	}
	return specifier
}

func isScript(file string) bool {
	return slices.Contains(scriptExtensions, path.Ext(file)) && !strings.HasSuffix(file, declarationFileEnd)
}
