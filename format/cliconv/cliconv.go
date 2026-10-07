// Package cliconv is the one definition of the command-line conventions the
// binaries of the repository share: how a request for help is recognised, what a
// usage text looks like, how the flags of a command are parsed and reported, and
// how the fields of an output line are joined. Every binary calls it, so that
// `-h`, `--help` and `help` behave the same in all of them and an output line
// never ends in a separator.
//
// A flag set made here writes nothing itself. A parse error comes back as a
// UsageError and a request for help as a HelpRequest; Handle turns either into
// the text and the exit code of the convention.
package cliconv

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Exit codes of the convention: a request for help succeeds, a command line that
// does not parse is a usage error.
const (
	ExitHelp  = 0
	ExitUsage = 2
)

// HelpCommand is the word that asks for help in the place of a command.
const HelpCommand = "help"

const (
	usagePrefix      = "usage: "
	flagPrefix       = "--"
	flagIndent       = "  "
	flagColumnGap    = "  "
	commandSeparator = "|"
	fieldSeparator   = " "
	flagsPlaceholder = "[flags]"
	flagsHeading     = "flags:"
	lineBreak        = "\n"
)

// helpArguments are the spellings of the flag that asks for help; the flag
// package takes `-h` and `-help` with one dash or two.
var helpArguments = map[string]bool{"-h": true, "--h": true, "-help": true, "--help": true}

// IsHelpArgument reports whether an argument asks for help.
func IsHelpArgument(argument string) bool { return helpArguments[argument] }

// IsHelpRequest reports whether the first argument of a command line asks for
// help, as a flag or as the word `help`.
func IsHelpRequest(arguments []string) bool {
	return len(arguments) > 0 && (arguments[0] == HelpCommand || IsHelpArgument(arguments[0]))
}

// HelpRequest is the error of a command line that asked for help; its text is
// what to print.
type HelpRequest struct{ Text string }

func (HelpRequest) Error() string { return "help requested" }

// UsageError is the error of a command line that does not parse; its text is the
// usage of the command it was meant for.
type UsageError struct {
	Err  error
	Text string
}

func (e UsageError) Error() string { return e.Err.Error() }

// Unwrap returns the parse error.
func (e UsageError) Unwrap() error { return e.Err }

// FlagSet is the set of flags of one command, with the arguments the command
// takes besides its flags, spelled as the usage shows them: `<ticket>`.
type FlagSet struct {
	*flag.FlagSet
	arguments string
}

// NewFlagSet builds the flag set of a command called name, which takes the given
// positional arguments (empty for none).
func NewFlagSet(name, arguments string) *FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	return &FlagSet{FlagSet: set, arguments: arguments}
}

// Usage is the text that describes the command: its synopsis, then each flag with
// its value and its default.
func (s *FlagSet) Usage() string {
	var out strings.Builder
	out.WriteString(usagePrefix + Line(s.Name(), s.arguments, flagsPlaceholder) + lineBreak)
	rows := flagRows(s.FlagSet)
	if len(rows) == 0 {
		return out.String()
	}
	out.WriteString(lineBreak + flagsHeading + lineBreak)
	width := 0
	for _, row := range rows {
		width = max(width, len(row.label))
	}
	for _, row := range rows {
		out.WriteString(flagIndent + row.label + strings.Repeat(fieldSeparator, width-len(row.label)) + flagColumnGap + row.description + lineBreak)
	}
	return out.String()
}

type flagRow struct{ label, description string }

func flagRows(set *flag.FlagSet) []flagRow {
	var rows []flagRow
	set.VisitAll(func(f *flag.Flag) {
		value, usage := flag.UnquoteUsage(f)
		label := flagPrefix + f.Name
		if value != "" {
			label += fieldSeparator + value
		}
		description := usage
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
			description += fmt.Sprintf(" (default %s)", f.DefValue)
		}
		rows = append(rows, flagRow{label: label, description: description})
	})
	return rows
}

// ParseArguments parses the flags that lead the arguments and returns what
// follows them. A request for help comes back as a HelpRequest and any other
// failure as a UsageError, both carrying the usage of the command.
func (s *FlagSet) ParseArguments(arguments []string) ([]string, error) {
	if err := s.Parse(arguments); err != nil {
		return nil, s.report(err)
	}
	return s.Args(), nil
}

// ParseInterleaved parses flags that may follow positional arguments, as
// `request submit <ticket> --nonce N` has them, and returns the positionals.
func (s *FlagSet) ParseInterleaved(arguments []string) ([]string, error) {
	var positionals []string
	for {
		rest, err := s.ParseArguments(arguments)
		if err != nil {
			return nil, err
		}
		if len(rest) == 0 {
			return positionals, nil
		}
		positionals = append(positionals, rest[0])
		arguments = rest[1:]
	}
}

func (s *FlagSet) report(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return HelpRequest{Text: s.Usage()}
	}
	return UsageError{Err: err, Text: s.Usage()}
}

// CommandsUsage is the usage text of a binary: its commands, in order.
func CommandsUsage(binary string, commands []string) string {
	return usagePrefix + binary + fieldSeparator + "<" + strings.Join(commands, commandSeparator) + ">" + lineBreak
}

// VerbsUsage is the usage text of a command that takes a verb.
func VerbsUsage(binary, command string, verbs []string) string {
	return usagePrefix + binary + fieldSeparator + command + fieldSeparator + "<" + strings.Join(verbs, commandSeparator) + ">" + lineBreak
}

// Handle gives the text and the exit code the convention prescribes for the
// error of a command: a request for help prints its text on stdout and succeeds,
// a usage error prints the error and the usage on stderr and exits with
// ExitUsage. It reports false for any other error, which the caller handles.
func Handle(err error, binary string, stdout, stderr io.Writer) (code int, handled bool) {
	var help HelpRequest
	if errors.As(err, &help) {
		_, _ = io.WriteString(stdout, help.Text)
		return ExitHelp, true
	}
	var usage UsageError
	if errors.As(err, &usage) {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n%s", binary, usage.Err, usage.Text)
		return ExitUsage, true
	}
	return 0, false
}

// Line joins the fields of an output line with one space and leaves out the empty
// ones, so that a field with nothing to say leaves no trailing or doubled space.
func Line(fields ...string) string {
	kept := make([]string, 0, len(fields))
	for _, field := range fields {
		if field != "" {
			kept = append(kept, field)
		}
	}
	return strings.Join(kept, fieldSeparator)
}
