package jvm

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/platform/witnesskit"
)

const (
	javaName   = "Java"
	kotlinName = "Kotlin"

	javaSourceSet   = "java"
	kotlinSourceSet = "kotlin"

	javaStatementEnd   = ";"
	kotlinStatementEnd = ""

	testAnnotationImport = "org.junit.jupiter.api.Test"
	displayNameImport    = "org.junit.jupiter.api.DisplayName"
	defaultPackageName   = "the default package"
)

// declaration is a class, interface, object, field or property of a source,
// reduced to its header.
type declaration struct {
	offset int
	name   string
	header string
}

// witnessLanguage is what differs between the languages witnesses are written
// in: how a signature is cut, which members are public, how a test class reads.
// It is the Strategy of the witness host over Java and Kotlin.
type witnessLanguage interface {
	name() string
	extension() string
	sourceSet() string
	// statementEnd closes a package clause and an import.
	statementEnd() string
	// header cuts a method or function down to its signature.
	header(definition string) string
	// exposed reports whether a signature is part of the public API.
	exposed(header string) bool
	// declarations lists the exposed types and constants of a source.
	declarations(source string) []declaration
	// methodDeclaration matches the declaration of a witness method by name.
	methodDeclaration(name string) *regexp.Regexp
	conventions(pkg string) string
}

var (
	javaLang   witnessLanguage = javaLanguage{}
	kotlinLang witnessLanguage = kotlinLanguage{}

	languages = []witnessLanguage{javaLang, kotlinLang}

	packageClause     = regexp.MustCompile(`(?m)^[ \t]*package[ \t]+([A-Za-z_][\w.]*)`)
	whitespaceRun     = regexp.MustCompile(`\s+`)
	hiddenModifier    = regexp.MustCompile(`\b(?:private|protected|internal)\b`)
	privateModifier   = regexp.MustCompile(`\bprivate\b`)
	publicModifier    = regexp.MustCompile(`\bpublic\b`)
	testAnnotation    = regexp.MustCompile(`@(?:Test|ParameterizedTest|RepeatedTest|TestFactory|TestTemplate|BeforeEach|AfterEach|BeforeAll|AfterAll)\b`)
	javaClassPattern  = regexp.MustCompile(`(?m)^[ \t]*((?:(?:public|protected|private|static|final|abstract|sealed|non-sealed|strictfp)[ \t]+)*)(?:class|interface|enum|record)[ \t]+(\w+)`)
	javaFieldPattern  = regexp.MustCompile(`(?m)^[ \t]*public[ \t]+[^(){};=\n]*?\b(\w+)[ \t]*(?:=|;)`)
	kotlinTypePattern = regexp.MustCompile(`(?m)^[ \t]*((?:(?:public|private|protected|internal|abstract|open|sealed|data|enum|annotation|inner|final|value|fun)[ \t]+)*)(?:class|interface|object)[ \t]+(\w+)`)
	kotlinValPattern  = regexp.MustCompile(`(?m)^(?:(?:public|const|lateinit)[ \t]+)*(?:val|var)[ \t]+(\w+)`)
)

// languageOf picks the language of a source file by its extension.
func languageOf(file string) (witnessLanguage, bool) {
	for _, candidate := range languages {
		if path.Ext(file) == candidate.extension() {
			return candidate, true
		}
	}
	return nil, false
}

// squash folds the whitespace of a signature to single spaces.
func squash(text string) string { return whitespaceRun.ReplaceAllString(strings.TrimSpace(text), " ") }

// shortest cuts a text at whichever of the terminators comes first outside
// brackets and quotes.
func shortest(text string, terminators ...byte) string {
	best := text
	for _, terminator := range terminators {
		if cut := witnesskit.Header(text, terminator); len(cut) < len(best) {
			best = cut
		}
	}
	return squash(best)
}

func beforeParameters(header string) string {
	if opening := strings.IndexByte(header, '('); opening >= 0 {
		return header[:opening]
	}
	return header
}

type javaLanguage struct{}

func (javaLanguage) name() string         { return javaName }
func (javaLanguage) extension() string    { return ".java" }
func (javaLanguage) sourceSet() string    { return javaSourceSet }
func (javaLanguage) statementEnd() string { return javaStatementEnd }

func (javaLanguage) header(definition string) string {
	return strings.TrimSpace(strings.TrimSuffix(shortest(definition, '{'), javaStatementEnd))
}

// exposed: a Java signature is public API when it says `public`.
func (javaLanguage) exposed(header string) bool {
	return publicModifier.MatchString(beforeParameters(header))
}

func (javaLanguage) declarations(source string) []declaration {
	var found []declaration
	for _, match := range javaClassPattern.FindAllStringSubmatchIndex(source, -1) {
		if publicModifier.MatchString(source[match[2]:match[3]]) {
			found = append(found, declaration{offset: match[0], name: source[match[4]:match[5]], header: shortest(source[match[0]:], '{')})
		}
	}
	for _, match := range javaFieldPattern.FindAllStringSubmatchIndex(source, -1) {
		found = append(found, declaration{offset: match[0], name: source[match[2]:match[3]], header: shortest(source[match[0]:], '=', ';')})
	}
	return found
}

func (javaLanguage) methodDeclaration(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^[ \t]*(?:(?:public|protected)[ \t]+)?void[ \t]+(` + regexp.QuoteMeta(name) + `)[ \t]*\(`)
}

func (javaLanguage) conventions(pkg string) string {
	return fmt.Sprintf("Each witness is a method `void <name>() { ... }` (add `throws Exception` when it needs it) of a JUnit 5 test class of package %s: "+
		"the public and package-private members of that package are in scope without an import. "+
		"Assert with JUnit 5 (`org.junit.jupiter.api.Assertions`), imported statically in \"imports\" one import per line, "+
		"for example `import static org.junit.jupiter.api.Assertions.assertEquals;`. "+
		"Compare numbers whose scale can differ, such as BigDecimal, with compareTo. "+
		"Do not annotate the method, do not import org.junit.jupiter.api.Test or DisplayName, and declare nothing besides the method. "+
		"A property draws its inputs from `new java.util.Random(<long constant>)`, never from the clock or an unseeded generator.", packageLabel(pkg))
}

type kotlinLanguage struct{}

func (kotlinLanguage) name() string         { return kotlinName }
func (kotlinLanguage) extension() string    { return ".kt" }
func (kotlinLanguage) sourceSet() string    { return kotlinSourceSet }
func (kotlinLanguage) statementEnd() string { return kotlinStatementEnd }

// header cuts a function at its block or at its expression body.
func (kotlinLanguage) header(definition string) string { return shortest(definition, '{', '=') }

// exposed: a Kotlin signature is public API unless it says private, protected or
// internal.
func (kotlinLanguage) exposed(header string) bool {
	return !hiddenModifier.MatchString(beforeParameters(header))
}

func (kotlinLanguage) declarations(source string) []declaration {
	var found []declaration
	for _, match := range kotlinTypePattern.FindAllStringSubmatchIndex(source, -1) {
		if !hiddenModifier.MatchString(source[match[2]:match[3]]) {
			found = append(found, declaration{offset: match[0], name: source[match[4]:match[5]], header: shortest(source[match[0]:], '{', '\n')})
		}
	}
	for _, match := range kotlinValPattern.FindAllStringSubmatchIndex(source, -1) {
		found = append(found, declaration{offset: match[0], name: source[match[2]:match[3]], header: shortest(source[match[0]:], '=', '\n')})
	}
	return found
}

func (kotlinLanguage) methodDeclaration(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^[ \t]*(?:public[ \t]+)?fun[ \t]+(` + regexp.QuoteMeta(name) + `)[ \t]*\(`)
}

func (kotlinLanguage) conventions(pkg string) string {
	return fmt.Sprintf("Each witness is a function `fun <name>() { ... }` of a JUnit 5 test class of package %s: "+
		"the public and internal top-level declarations of that package are in scope without an import. "+
		"Assert with JUnit 5 (`org.junit.jupiter.api.Assertions`), imported in \"imports\" one import per line, "+
		"for example `import org.junit.jupiter.api.Assertions.assertEquals`. "+
		"Do not annotate the function, do not import org.junit.jupiter.api.Test or DisplayName, and declare nothing besides the function. "+
		"A property draws its inputs from `kotlin.random.Random(<Int constant>)`, never from the clock or an unseeded generator.", packageLabel(pkg))
}

func packageLabel(pkg string) string {
	if pkg == "" {
		return defaultPackageName
	}
	return "`" + pkg + "`"
}
