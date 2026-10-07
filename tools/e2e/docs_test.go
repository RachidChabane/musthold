package e2e_test

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Locations of the documentation the tests read, relative to the repository.
const (
	readmeFile          = "README.md"
	contributingFile    = "CONTRIBUTING.md"
	architectureFile    = "docs/architecture.md"
	usageDirectory      = "docs/usage"
	moduleReadme        = "README.md"
	markdownSuffix      = ".md"
	fenceMarker         = "```"
	commandPromptPrefix = "$ "
)

// Requirements the documentation tests verify.
const (
	verifiesDocumentedCommands = " [verifies SP-1400~1]"
	verifiesDocumentedFlags    = " [verifies SP-1401~1]"
	verifiesDocumentedLinks    = " [verifies SP-1402~1]"
)

// How a binary reports its commands and the verbs of a command.
const (
	canaryFlag          = "zz-undefined"
	undefinedFlag       = "flag provided but not defined: -"
	invalidValueFor     = "for flag -"
	verbsPrefix         = "needs one of "
	verbSeparators      = `,\s*|\s+or\s+`
	wordPattern         = `^[a-z][a-z0-9-]*$`
	commandsPattern     = `<([a-z0-9|-]+)>`
	flagPattern         = `^-{1,2}([a-z][a-z-]*)(?:=.*)?$`
	wordsPerCommand     = 2
	commandSeparate     = "|"
	assignedValue       = "=1"
	flagDashes          = "--"
	emptyStateDirectory = "state"
	emptyCacheDirectory = "cache"
	homeVariable        = "HOME"
	stateVariable       = "XDG_STATE_HOME"
	cacheVariable       = "XDG_CACHE_HOME"
)

// documentedModules are the modules whose directory holds a README.
var documentedModules = []string{"format", "discpower", "alignment", "completeness", "ledger", "verify", "s1relay", "umbrella"}

// documentedBinaries are the binaries the documentation may name.
var documentedBinaries = []string{dpBinary, alignmentBinary, completenessBinary, ledgerBinary, verifyBinary, s1Binary}

// verbCommands are the commands of the proof modules that take a verb.
var verbCommands = []string{"request", "systemone", "host", "hook"}

var (
	wordRegexp     = regexp.MustCompile(wordPattern)
	commandsRegexp = regexp.MustCompile(commandsPattern)
	flagRegexp     = regexp.MustCompile(flagPattern)
	verbSplitter   = regexp.MustCompile(verbSeparators)
	linkRegexp     = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	inlineRegexp   = regexp.MustCompile("`((?:" + strings.Join(quoteAll(documentedBinaries), "|") + ")(?:\\s[^`]*)?)`")
	headingRegexp  = regexp.MustCompile(`^#{1,6}\s+(.*)$`)
	slugDropRegexp = regexp.MustCompile(`[^a-z0-9 _-]`)
)

func quoteAll(values []string) []string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = regexp.QuoteMeta(value)
	}
	return quoted
}

// invocation is one command line of a binary the documentation shows.
type invocation struct {
	where  string
	binary string
	words  []string
	flags  []string
}

// repositoryRoot is the absolute root of the repository.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(repositoryDir)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// userDocuments lists the user documentation: the README, the module READMEs and
// the pages under docs/usage.
func userDocuments(t *testing.T, root string) []string {
	t.Helper()
	documents := []string{filepath.Join(root, readmeFile)}
	for _, module := range documentedModules {
		documents = append(documents, filepath.Join(root, module, moduleReadme))
	}
	pages, err := filepath.Glob(filepath.Join(root, usageDirectory, "*"+markdownSuffix))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Fatalf("no page under %s", usageDirectory)
	}
	return append(documents, pages...)
}

// readLines returns the lines of a file.
func readLines(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

// invocationsOf finds the command lines of a binary in a document: the lines of
// fenced blocks that start with a binary, and the inline code spans that do.
func invocationsOf(t *testing.T, root, path string) []invocation {
	t.Helper()
	var found []invocation
	inFence := false
	for index, line := range readLines(t, path) {
		where := fmt.Sprintf("%s:%d", relativeTo(root, path), index+1)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, fenceMarker) {
			inFence = !inFence
			continue
		}
		var candidates []string
		if inFence {
			candidates = []string{strings.TrimPrefix(trimmed, commandPromptPrefix)}
		} else {
			for _, match := range inlineRegexp.FindAllStringSubmatch(line, -1) {
				candidates = append(candidates, match[1])
			}
		}
		for _, candidate := range candidates {
			if parsed, ok := parseInvocation(where, candidate); ok {
				found = append(found, parsed)
			}
		}
	}
	return found
}

func relativeTo(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}

// parseInvocation reads a command line that starts with a documented binary: up to
// two command words, then the flags, whatever the placeholders and brackets around
// them.
func parseInvocation(where, text string) (invocation, bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 || !slices.Contains(documentedBinaries, fields[0]) {
		return invocation{}, false
	}
	parsed := invocation{where: where, binary: fields[0]}
	rest := fields[1:]
	for len(rest) > 0 && len(parsed.words) < wordsPerCommand && wordRegexp.MatchString(rest[0]) {
		parsed.words = append(parsed.words, rest[0])
		rest = rest[1:]
	}
	for _, token := range rest {
		token = strings.Trim(token, "[],")
		if match := flagRegexp.FindStringSubmatch(token); match != nil {
			parsed.flags = append(parsed.flags, match[1])
		}
	}
	return parsed, true
}

// run executes a built binary with arguments in an empty directory with an empty
// home and returns what it printed.
func run(t *testing.T, directory, binary string, arguments ...string) string {
	t.Helper()
	home := t.TempDir()
	command := exec.Command(filepath.Join(directory, binary), arguments...)
	command.Dir = home
	command.Env = append(os.Environ(), homeVariable+"="+home,
		stateVariable+"="+filepath.Join(home, emptyStateDirectory), cacheVariable+"="+filepath.Join(home, emptyCacheDirectory))
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	_ = command.Run()
	return output.String()
}

// commandsOf reads the commands a binary names when it is run with no argument:
// the `<a|b|c>` list of the proof modules and the relay, or the lines of the usage
// text of the ledger and the verifier.
func commandsOf(t *testing.T, directory, binary string) []string {
	t.Helper()
	usage := run(t, directory, binary)
	if match := commandsRegexp.FindStringSubmatch(usage); match != nil {
		return strings.Split(match[1], commandSeparate)
	}
	var commands []string
	for _, line := range strings.Split(usage, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && fields[0] == binary && wordRegexp.MatchString(fields[1]) && !slices.Contains(commands, fields[1]) {
			commands = append(commands, fields[1])
		}
	}
	if len(commands) == 0 {
		t.Fatalf("%s names no command when run with no argument: %q", binary, usage)
	}
	return commands
}

// verbsOf reads the verbs of a command: a proof module names them when the command
// is run with none, the ledger names them in its usage text.
func verbsOf(t *testing.T, directory, binary, command string) []string {
	t.Helper()
	if slices.Contains(verbCommands, command) {
		output := run(t, directory, binary, command)
		_, list, found := strings.Cut(output, verbsPrefix)
		if !found {
			t.Fatalf("%s %s names no verb when run with none: %q", binary, command, output)
		}
		return verbSplitter.Split(strings.TrimSpace(strings.SplitN(list, "\n", 2)[0]), -1)
	}
	var verbs []string
	for _, line := range strings.Split(run(t, directory, binary), "\n") {
		fields := strings.Fields(line)
		if len(fields) > wordsPerCommand && fields[0] == binary && fields[1] == command && wordRegexp.MatchString(fields[2]) && !slices.Contains(verbs, fields[2]) {
			verbs = append(verbs, fields[2])
		}
	}
	return verbs
}

// acceptsFlag reports whether a command parses a flag: the command line ends with a
// flag no command defines, so parsing fails before anything runs, and the failure
// names the first flag it does not know.
func acceptsFlag(t *testing.T, directory string, documented invocation, flag string) (bool, string) {
	t.Helper()
	arguments := append(slices.Clone(documented.words), flagDashes+flag+assignedValue, flagDashes+canaryFlag)
	output := run(t, directory, documented.binary, arguments...)
	reachedCanary := strings.Contains(output, undefinedFlag+canaryFlag)
	failedOnValue := strings.Contains(output, invalidValueFor+flag+":")
	return reachedCanary || failedOnValue, output
}

// TestDocumentedCommandsExist checks the commands and flags the user documentation
// names against the built binaries.
func TestDocumentedCommandsExist(t *testing.T) {
	root := requireMemory(t, []string{"SP-1400~1", "SP-1401~1"})
	directory := productBinaries(t)
	commands := map[string][]string{}
	for _, binary := range documentedBinaries {
		commands[binary] = commandsOf(t, directory, binary)
	}
	var invocations []invocation
	for _, document := range userDocuments(t, root) {
		invocations = append(invocations, invocationsOf(t, root, document)...)
	}
	if len(invocations) == 0 {
		t.Fatal("the documentation shows no command of a spec-proof binary")
	}

	t.Run("every command and verb the documentation names exists in the binary"+verifiesDocumentedCommands, func(t *testing.T) {
		for _, documented := range invocations {
			if len(documented.words) == 0 {
				continue
			}
			command := documented.words[0]
			if !slices.Contains(commands[documented.binary], command) {
				t.Errorf("%s: %s has no command %q (it has %v)", documented.where, documented.binary, command, commands[documented.binary])
				continue
			}
			verbs := verbsOf(t, directory, documented.binary, command)
			switch {
			case len(documented.words) == 1:
			case !slices.Contains(verbs, documented.words[1]):
				t.Errorf("%s: %s %s has no verb %q (it has %v)", documented.where, documented.binary, command, documented.words[1], verbs)
			}
		}
	})

	t.Run("every flag the documentation names is accepted by its command"+verifiesDocumentedFlags, func(t *testing.T) {
		for _, documented := range invocations {
			for _, flag := range documented.flags {
				if accepted, output := acceptsFlag(t, directory, documented, flag); !accepted {
					t.Errorf("%s: %s %s does not accept --%s: %q", documented.where, documented.binary, strings.Join(documented.words, " "), flag, strings.TrimSpace(output))
				}
			}
		}
	})
}

// TestDocumentedLinksResolve checks that every relative link of the documentation
// reaches a file or a directory of the repository, and that a link to a heading of
// a Markdown page reaches a heading that page has.
func TestDocumentedLinksResolve(t *testing.T) {
	root := repositoryRoot(t)
	documents := append(userDocuments(t, root), filepath.Join(root, contributingFile), filepath.Join(root, architectureFile))
	t.Run("every relative link and heading link of the documentation resolves"+verifiesDocumentedLinks, func(t *testing.T) {
		checked := 0
		for _, document := range documents {
			for _, link := range linksOf(t, document) {
				checked++
				if problem := resolveLink(t, document, link.target); problem != "" {
					t.Errorf("%s:%d: %s: %s", relativeTo(root, document), link.line, link.target, problem)
				}
			}
		}
		if checked == 0 {
			t.Fatal("the documentation holds no relative link")
		}
	})
}

type link struct {
	line   int
	target string
}

// linksOf lists the relative links of a Markdown file outside fenced blocks.
func linksOf(t *testing.T, path string) []link {
	t.Helper()
	var links []link
	inFence := false
	for index, line := range readLines(t, path) {
		if strings.HasPrefix(strings.TrimSpace(line), fenceMarker) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, match := range linkRegexp.FindAllStringSubmatch(line, -1) {
			if isRelative(match[1]) {
				links = append(links, link{line: index + 1, target: match[1]})
			}
		}
	}
	return links
}

func isRelative(target string) bool {
	return !strings.Contains(target, "://") && !strings.HasPrefix(target, "mailto:")
}

// resolveLink returns what is wrong with a link of a document, or "".
func resolveLink(t *testing.T, document, target string) string {
	t.Helper()
	file, fragment, _ := strings.Cut(target, "#")
	destination := document
	if file != "" {
		destination = filepath.Join(filepath.Dir(document), filepath.FromSlash(file))
	}
	info, err := os.Stat(destination)
	if err != nil {
		return "the file does not exist"
	}
	if fragment == "" || info.IsDir() || !strings.HasSuffix(destination, markdownSuffix) {
		return ""
	}
	if !slices.Contains(headingSlugs(t, destination), fragment) {
		return "the page has no such heading"
	}
	return ""
}

// headingSlugs lists the anchors of the headings of a Markdown file, spelled as
// GitHub spells them: lower case, punctuation dropped, spaces as hyphens.
func headingSlugs(t *testing.T, path string) []string {
	t.Helper()
	var slugs []string
	inFence := false
	for _, line := range readLines(t, path) {
		if strings.HasPrefix(strings.TrimSpace(line), fenceMarker) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if match := headingRegexp.FindStringSubmatch(line); match != nil {
			text := slugDropRegexp.ReplaceAllString(strings.ToLower(strings.TrimSpace(match[1])), "")
			slugs = append(slugs, strings.ReplaceAll(text, " ", "-"))
		}
	}
	return slugs
}
