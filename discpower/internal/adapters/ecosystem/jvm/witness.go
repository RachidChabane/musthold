package jvm

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/discpower/internal/platform/witnesskit"
)

var _ ports.Witnessing = (*Strategy)(nil)

const (
	witnessClassPrefix  = "MustholdWitness"
	witnessClassSuffix  = "Test"
	witnessDigestLength = 12
	testSourceSet       = "src/test"
	memberIndent        = "    "
	testAnnotationName  = "@Test"
	displayNameFormat   = "@DisplayName(%q)"
	importKeyword       = "import"
	languageFormat      = "%s, JUnit 5 on %s"
	mavenDisplay        = "Maven"
	gradleDisplay       = "Gradle"
)

// buildDirectories hold build outputs and tool state, never sources.
var buildDirectories = []string{".git", ".gradle", ".kotlin", "build", "target", "node_modules"}

// mainSource matches a source file of a main source set, capturing the module
// directory it belongs to.
var mainSource = regexp.MustCompile(`^(.*?)src/main/(?:java|kotlin)/`)

// strippedAnnotation matches a line a writer annotated its test with.
var strippedAnnotation = regexp.MustCompile(`(?m)^[ \t]*@Test[ \t]*\n`)

// witnessTarget is where the witnesses of a project are placed: the module
// directory, the package of the code under test and the language of its sources.
type witnessTarget struct {
	moduleRoot string
	pkg        string
	language   witnessLanguage
}

// directory is the directory of the witness class, under the test source set.
func (t witnessTarget) directory() string {
	return path.Join(t.moduleRoot, testSourceSet, t.language.sourceSet(), strings.ReplaceAll(t.pkg, packageSeparator, "/"))
}

// targetMemo keeps the target a brief chose, so that the layout that follows
// places the witnesses where the brief said.
type targetMemo struct {
	mu     sync.Mutex
	target *witnessTarget
}

func (m *targetMemo) remember(target witnessTarget) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.target = &target
}

func (m *targetMemo) recall() (witnessTarget, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.target == nil {
		return witnessTarget{}, false
	}
	return *m.target, true
}

// Witnesses is the witness host of the Maven and Gradle projects, in Java and in
// Kotlin.
func (s *Strategy) Witnesses() ports.WitnessHost { return witnessHost{strategy: s} }

type witnessHost struct {
	strategy *Strategy
}

// sourceUnit is one source of the code under test with its signatures.
type sourceUnit struct {
	target  witnessTarget
	entries []witnesskit.Entry
}

// Brief lists the public signatures of the sources the spec mentions, without
// bodies, the helpers of every test file, and places the witnesses in the
// package the spec mentions most.
func (h witnessHost) Brief(project fs.FS, sources []mutation.SourceFile, specText string) (ports.WitnessBrief, error) {
	units, err := h.units(sources)
	if err != nil {
		return ports.WitnessBrief{}, err
	}
	mentioned := witnesskit.Mentions(specText)
	var entries []witnesskit.Entry
	for _, unit := range units {
		entries = append(entries, unit.entries...)
	}
	target, found := mostMentioned(units, mentioned)
	if !found {
		if target, found = h.firstTarget(project); !found {
			return ports.WitnessBrief{}, fmt.Errorf("no JVM source to place the witnesses beside")
		}
	}
	h.strategy.witnessTarget.remember(target)
	helpers, err := h.helpers(project)
	if err != nil {
		return ports.WitnessBrief{}, err
	}
	return ports.WitnessBrief{
		Language:    fmt.Sprintf(languageFormat, target.language.name(), displayNames[h.strategy.Name()]),
		Conventions: target.language.conventions(target.pkg),
		Interface:   witnesskit.Choose(entries, mentioned),
		Helpers:     helpers,
	}, nil
}

var displayNames = map[ecosystem.Name]string{ecosystem.Maven: mavenDisplay, ecosystem.Gradle: gradleDisplay}

// units reads the signatures of each Java and Kotlin source of a main source set.
func (h witnessHost) units(sources []mutation.SourceFile) ([]sourceUnit, error) {
	var units []sourceUnit
	for _, source := range sources {
		language, known := languageOf(source.Path)
		if !known || testSourceFile.MatchString(source.Path) {
			continue
		}
		pkg := packageOf(source.Content)
		entries, err := h.publicEntries(signatureSource{language: language, pkg: pkg, content: source.Content})
		if err != nil {
			return nil, fmt.Errorf("list the signatures of %q: %w", source.Path, err)
		}
		units = append(units, sourceUnit{target: witnessTarget{moduleRoot: moduleRootOf(source.Path), pkg: pkg, language: language}, entries: entries})
	}
	return units, nil
}

func moduleRootOf(file string) string {
	if match := mainSource.FindStringSubmatch(file); match != nil {
		return match[1]
	}
	return ""
}

// mostMentioned is the target of the source with the most entries the spec
// mentions, the first source on a tie.
func mostMentioned(units []sourceUnit, mentioned func(string) bool) (witnessTarget, bool) {
	var best witnessTarget
	bestCount := -1
	for _, unit := range units {
		count := 0
		for _, entry := range unit.entries {
			if slices.ContainsFunc(entry.Names, mentioned) {
				count++
			}
		}
		if count > bestCount {
			best, bestCount = unit.target, count
		}
	}
	return best, bestCount >= 0
}

// firstTarget is the target of the first source of a main source set of the
// project, in path order.
func (h witnessHost) firstTarget(project fs.FS) (witnessTarget, bool) {
	var target witnessTarget
	found := false
	_ = fs.WalkDir(project, ".", func(file string, entry fs.DirEntry, err error) error {
		switch language, known := languageOf(file); {
		case err != nil || found:
			return fs.SkipAll
		case entry.IsDir() && slices.Contains(buildDirectories, entry.Name()):
			return fs.SkipDir
		case entry.IsDir() || !known || mainSource.FindStringSubmatch(file) == nil:
			return nil
		default:
			content, readErr := fs.ReadFile(project, file)
			if readErr != nil {
				return nil
			}
			target = witnessTarget{moduleRoot: moduleRootOf(file), pkg: packageOf(content), language: language}
			found = true
			return fs.SkipAll
		}
	})
	return target, found
}

// Layout writes a set as one test class in the package of the code under test,
// under the test source set, each witness renamed after its short id, annotated
// as a JUnit 5 test and named by its renamed name.
func (h witnessHost) Layout(project fs.FS, set witness.Set) (ports.WitnessFile, error) {
	target, found := h.strategy.witnessTarget.recall()
	if !found {
		if target, found = h.firstTarget(project); !found {
			return ports.WitnessFile{}, fmt.Errorf("no JVM source to place the witnesses beside")
		}
	}
	language := target.language
	className := witnessClassPrefix + set.Digest()[:witnessDigestLength] + witnessClassSuffix
	file := path.Join(target.directory(), className+language.extension())
	classname := qualify(target.pkg, []string{className})

	var content strings.Builder
	if target.pkg != "" {
		content.WriteString("package " + target.pkg + language.statementEnd() + "\n\n")
	}
	for _, line := range importLines(language, set.Imports) {
		content.WriteString(line + "\n")
	}
	fmt.Fprintf(&content, "\nclass %s {\n", className)
	tests := map[string]ecosystem.BoundTest{}
	for _, w := range set.Witnesses {
		renamed := witnesskit.RenamedName("", w.ShortID())
		code, declared := witnesskit.Rename(strippedAnnotation.ReplaceAllString(w.Code, ""), language.methodDeclaration(w.Name), renamed)
		if !declared {
			continue
		}
		content.WriteString("\n" + memberIndent + testAnnotationName + "\n" + memberIndent + fmt.Sprintf(displayNameFormat, renamed) + "\n")
		indented := indent(strings.TrimRight(code, "\n"))
		declaration := language.methodDeclaration(renamed).FindStringIndex(indented)
		line := witnesskit.LineOf(content.String(), content.Len()) + witnesskit.LineOf(indented, declaration[0]) - 1
		content.WriteString(indented + "\n")
		tests[w.ID] = ecosystem.NewBoundTest(classname, renamed, file, line)
	}
	content.WriteString("}\n")
	return ports.WitnessFile{Path: file, Content: []byte(content.String()), Tests: tests}, nil
}

// indent indents every non-blank line of a block by one level.
func indent(block string) string {
	lines := strings.Split(block, "\n")
	for index, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[index] = memberIndent + line
		}
	}
	return strings.Join(lines, "\n")
}

// importLines normalizes the imports a writer listed into import statements of
// the language, adds the JUnit annotations the layout uses and drops duplicates.
func importLines(language witnessLanguage, imports string) []string {
	var lines []string
	add := func(raw string) {
		line := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ";"))
		if line == "" {
			return
		}
		if !strings.HasPrefix(line, importKeyword+" ") {
			line = importKeyword + " " + line
		}
		if line += language.statementEnd(); !slices.Contains(lines, line) {
			lines = append(lines, line)
		}
	}
	for _, raw := range strings.Split(imports, "\n") {
		add(raw)
	}
	add(testAnnotationImport)
	add(displayNameImport)
	return lines
}
