package suiterun

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
)

// failureOutputBytes bounds the end of the output a runner failure quotes.
const failureOutputBytes = 2048

// Failure describes how a runner's process ended when it did not exit with
// one of the clean exit codes: its command, the deadline it ran past, the
// signal or the exit code that ended it, and the end of what it printed, its
// standard error last; empty for a clean exit.
func Failure(command ports.Command, result ports.CommandResult, cleanExits ...int) string {
	var ending string
	switch {
	case result.TimedOut:
		ending = fmt.Sprintf("ran past its deadline of %s", command.Timeout.Round(time.Second))
	case result.Signal != nil:
		ending = fmt.Sprintf("was ended by the signal %q", result.Signal.String())
	case slices.Contains(cleanExits, result.ExitCode):
		return ""
	default:
		ending = fmt.Sprintf("exited %d", result.ExitCode)
	}
	description := fmt.Sprintf("`%s` %s", strings.Join(command.Argv, " "), ending)
	if output := lastBytes(strings.TrimSpace(string(result.Stdout)+"\n"+string(result.Stderr)), failureOutputBytes); output != "" {
		description += ":\n" + output
	}
	return description
}

// lastBytes is the end of the text, at most limit bytes of it, cut at a line.
func lastBytes(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	tail := text[len(text)-limit:]
	if newline := strings.IndexByte(tail, '\n'); newline >= 0 {
		tail = tail[newline+1:]
	}
	return tail
}
