package jvm

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/platform/witnesskit"
)

const (
	qualifiedSeparator = "."
	ownerSeparator     = "."
	signatureSeparator = ": "
)

// testSourceFile matches the files of a test source set, whatever their name.
var testSourceFile = regexp.MustCompile(`(?:^|/)src/test/(?:java|kotlin)/.+\.(?:java|kt)$`)

// located is a signature and where its declaration starts, to keep source order.
type located struct {
	offset int
	entry  witnesskit.Entry
}

// signatureSource is one file whose signatures are listed.
type signatureSource struct {
	language witnessLanguage
	pkg      string
	content  []byte
}

// publicEntries lists the signatures of the public methods, constructors,
// types and constants of a source, without a body.
func (h witnessHost) publicEntries(file signatureSource) ([]witnesskit.Entry, error) {
	found, err := h.methodEntries(file, func(header string) bool { return file.language.exposed(header) })
	if err != nil {
		return nil, err
	}
	for _, item := range file.language.declarations(string(file.content)) {
		found = append(found, located{offset: item.offset, entry: witnesskit.Entry{
			Names: []string{item.name}, Signature: qualify(file.pkg, nil) + signatureSeparator + item.header,
		}})
	}
	return ordered(found), nil
}

// helperEntries lists the signatures of the methods of a test file that no
// runner calls: neither tests nor lifecycle methods, and not private.
func (h witnessHost) helperEntries(file signatureSource) ([]witnesskit.Entry, error) {
	found, err := h.methodEntries(file, func(header string) bool {
		return !testAnnotation.MatchString(header) && !privateModifier.MatchString(beforeParameters(header))
	})
	if err != nil {
		return nil, err
	}
	return ordered(found), nil
}

// methodEntries renders each method the locator finds whose header the filter
// keeps; a constructor of a class is listed under its class.
func (h witnessHost) methodEntries(file signatureSource, keep func(header string) bool) ([]located, error) {
	definitions, err := h.strategy.deps.Locator.Locate("source"+file.language.extension(), file.content)
	if err != nil {
		return nil, fmt.Errorf("locate the definitions: %w", err)
	}
	var found []located
	for _, definition := range definitions {
		if definition.Kind == ecosystem.KindFunction && strings.Contains(definition.Qualname, qualifiedSeparator) {
			continue
		}
		text, err := definition.Span.Slice(file.content)
		if err != nil {
			return nil, err
		}
		header := file.language.header(string(text))
		if !keep(header) {
			continue
		}
		parts := strings.Split(definition.Qualname, qualifiedSeparator)
		found = append(found, located{offset: definition.Span.Start, entry: witnesskit.Entry{
			Names: parts, Signature: qualify(file.pkg, parts[:len(parts)-1]) + signatureSeparator + header,
		}})
	}
	return found, nil
}

// qualify is the package and the owners that qualify a member.
func qualify(pkg string, owners []string) string {
	parts := owners
	if pkg != "" {
		parts = append([]string{pkg}, owners...)
	}
	return strings.Join(parts, ownerSeparator)
}

func ordered(found []located) []witnesskit.Entry {
	sort.SliceStable(found, func(i, j int) bool { return found[i].offset < found[j].offset })
	entries := make([]witnesskit.Entry, len(found))
	for index, item := range found {
		entries[index] = item.entry
	}
	return entries
}

// helpers lists the signatures of the helpers of every test file of the
// project: the files the layout names tests and the files of a test source set.
func (h witnessHost) helpers(project fs.FS) ([]string, error) {
	files, err := witnesskit.TestFiles(project, h.strategy.Layout(), testSourceFile.MatchString)
	if err != nil {
		return nil, err
	}
	var helpers []string
	for _, file := range files {
		language, known := languageOf(file)
		if !known {
			continue
		}
		content, err := fs.ReadFile(project, file)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", file, err)
		}
		entries, err := h.helperEntries(signatureSource{language: language, pkg: packageOf(content), content: content})
		if err != nil {
			continue
		}
		for _, entry := range entries {
			helpers = append(helpers, entry.Signature)
		}
	}
	return helpers, nil
}

// packageOf reads the package clause of a source, empty for the default package.
func packageOf(content []byte) string {
	if match := packageClause.FindSubmatch(content); match != nil {
		return string(match[1])
	}
	return ""
}
